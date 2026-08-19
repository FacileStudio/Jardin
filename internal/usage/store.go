package usage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func machineDir(dataDir, machine string) string {
	return filepath.Join(dataDir, "usage", machine)
}

func currentPath(dataDir, machine string) string {
	return filepath.Join(machineDir(dataDir, machine), "current.json")
}

func historyPath(dataDir, machine string, t time.Time) string {
	return filepath.Join(machineDir(dataDir, machine), t.UTC().Format("2006-01")+".jsonl")
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".current-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Record publishes the snapshot as this machine's current.json and, when the
// throttle allows, appends it to the month's history shard. One writer per
// machine, so both files ride the normal file sync without conflicts.
// Record persists a snapshot as the current observation and appends it to
// history. Both ingest paths write the same file, and the OAuth path can
// answer from a five-minute-old cache, so the newer observation always wins;
// history still gets the sample either way, because the shards are an audit
// trail of what was observed when, not a view of the present.
func Record(dataDir, machine string, s Snapshot) error {
	if machine == "" {
		return errors.New("machine name required")
	}
	s.Machine = machine
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now()
	}
	s.UpdatedAt = s.UpdatedAt.UTC().Truncate(time.Second)
	if s.Windows == nil {
		s.Windows = []Window{}
	}
	sortWindows(s.Windows)

	if stored, ok := ReadOne(dataDir, machine); !ok || s.UpdatedAt.After(stored.UpdatedAt) {
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := writeAtomic(currentPath(dataDir, machine), data); err != nil {
			return err
		}
	}

	path := historyPath(dataDir, machine, s.UpdatedAt)
	last, hasLast := lastSample(path)
	if hasLast && !worthRecording(last, s) {
		return nil
	}
	line, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// worthRecording keeps the history readable: a sample lands when a window has
// moved at least HistoryDelta points, when the window set changed, or when the
// last sample has aged past HistoryThrottle.
func worthRecording(last, next Snapshot) bool {
	if next.UpdatedAt.Sub(last.UpdatedAt) > HistoryThrottle {
		return true
	}
	if len(last.Windows) != len(next.Windows) {
		return true
	}
	previous := make(map[string]float64, len(last.Windows))
	for _, w := range last.Windows {
		previous[w.Key] = w.UsedPercentage
	}
	for _, w := range next.Windows {
		before, ok := previous[w.Key]
		if !ok {
			return true
		}
		if delta := w.UsedPercentage - before; delta >= HistoryDelta || delta <= -HistoryDelta {
			return true
		}
	}
	return false
}

// lastSample reads the final JSON line of a shard without loading the whole
// file: the status line calls this on nearly every keystroke.
func lastSample(path string) (Snapshot, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return Snapshot{}, false
	}
	const window = 8 << 10
	size := info.Size()
	offset := int64(0)
	if size > window {
		offset = size - window
	}
	buf := make([]byte, size-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return Snapshot{}, false
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var s Snapshot
		if json.Unmarshal([]byte(lines[i]), &s) == nil && !s.UpdatedAt.IsZero() {
			return s, true
		}
	}
	return Snapshot{}, false
}

func machines(dataDir string) []string {
	entries, err := os.ReadDir(filepath.Join(dataDir, "usage"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}
