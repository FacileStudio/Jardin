package sessions

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// collectNacelle tails every Nacelle transcript under nacelleDir/sessions,
// resuming from per-file byte offsets kept in state. Only KindTurn records
// carry countable usage; KindDone repeats the run total and would double
// every figure.
func collectNacelle(nacelleDir string, state *ScanState) ([]Event, error) {
	pattern := filepath.Join(nacelleDir, "sessions", "*.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		fs := state.Files[path]
		if fs == nil {
			fs = &FileState{}
			state.Files[path] = fs
		}
		if info.Size() == fs.Size {
			continue
		}
		if info.Size() < fs.Offset {
			fs.Offset = 0
		}
		evs, newOffset, err := tailNacelle(path, fs.Offset)
		if err != nil {
			continue
		}
		fs.Offset = newOffset
		fs.Size = info.Size()
		events = append(events, evs...)
	}
	return events, nil
}

type nacelleRecord struct {
	Kind  string      `json:"kind"`
	TS    string      `json:"ts"`
	Model string      `json:"model,omitempty"`
	Usage *nacelleUse `json:"usage,omitempty"`
}

type nacelleUse struct {
	Input      int64   `json:"input_tokens"`
	Output     int64   `json:"output_tokens"`
	CacheRead  int64   `json:"cache_read_tokens"`
	CacheWrite int64   `json:"cache_creation_tokens"`
	Cost       float64 `json:"cost"`
}

func tailNacelle(path string, offset int64) ([]Event, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}

	var events []Event
	offsetNow := offset
	reader := bufio.NewReaderSize(f, 256*1024)
	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil && !completeRecord(raw) {
			break
		}
		offsetNow += int64(len(raw))
		if ev, ok := nacelleEvent(raw); ok {
			events = append(events, ev)
		}
		if err != nil {
			break
		}
	}
	return events, offsetNow, nil
}

// completeRecord reports whether bytes read without a closing newline are
// nevertheless a whole record. A session file whose last line never got its
// newline — the writer stopped, or the process died after the record — would
// otherwise never be counted: the offset stays before it and collectNacelle
// skips the file for good once its size stops changing. A genuinely torn write
// cannot parse as JSON, so it still waits for the rest.
func completeRecord(raw []byte) bool {
	var probe json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &probe) == nil
}

func nacelleEvent(raw []byte) (Event, bool) {
	var rec nacelleRecord
	if json.Unmarshal(raw, &rec) != nil || rec.Kind != "turn" || rec.Usage == nil {
		return Event{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, rec.TS)
	if err != nil {
		return Event{}, false
	}
	return Event{
		Time:       ts,
		Agent:      "nacelle",
		Model:      rec.Model,
		TokensIn:   rec.Usage.Input,
		TokensOut:  rec.Usage.Output,
		CacheRead:  rec.Usage.CacheRead,
		CacheWrite: rec.Usage.CacheWrite,
		CostTotal:  rec.Usage.Cost,
	}, true
}
