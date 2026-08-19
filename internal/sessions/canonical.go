package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CanonicalEvent is one session event in the transport shape: type, role and
// timestamp, plus the agent/machine/project it belongs to and optional usage.
type CanonicalEvent struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Timestamp string          `json:"timestamp"`
	Agent     string          `json:"agent"`
	Machine   string          `json:"machine"`
	Project   string          `json:"project"`
	Branch    string          `json:"branch,omitempty"`
	Model     string          `json:"model,omitempty"`
	Provider  string          `json:"provider,omitempty"`
	Usage     *CanonicalUsage `json:"usage,omitempty"`
}

// CanonicalUsage is the token accounting attached to a canonical event.
type CanonicalUsage struct {
	Input       int64          `json:"input"`
	Output      int64          `json:"output"`
	CacheRead   int64          `json:"cacheRead"`
	CacheWrite  int64          `json:"cacheWrite"`
	Reasoning   int64          `json:"reasoning,omitempty"`
	TotalTokens int64          `json:"totalTokens,omitempty"`
	Cost        *CanonicalCost `json:"cost,omitempty"`
}

// CanonicalCost is the monetary cost of a canonical event.
type CanonicalCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

func eventsDir(dataDir string) string {
	return filepath.Join(dataDir, "events")
}

func collectCanonical(dataDir string, state *ScanState, resolve func(cwd string) string) ([]Event, error) {
	pattern := filepath.Join(eventsDir(dataDir), "*", "*.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil
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

		evs, newOffset, err := tailCanonicalFile(path, fs.Offset, resolve)
		if err != nil {
			continue
		}
		fs.Offset = newOffset
		fs.Size = info.Size()
		events = append(events, evs...)
	}
	return events, nil
}

// canonicalEventFrom decodes one canonical line into an Event. It reports
// false for anything that is not an assistant message carrying usage, and for
// a timestamp it cannot parse, since a block's bounds come from that instant.
func canonicalEventFrom(raw []byte, agentDir string, resolve func(cwd string) string) (Event, bool) {
	var ce CanonicalEvent
	if err := json.Unmarshal(raw, &ce); err != nil {
		return Event{}, false
	}
	if ce.Type != "message" || ce.Role != "assistant" || ce.Usage == nil {
		return Event{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, ce.Timestamp)
	if err != nil {
		return Event{}, false
	}
	agent := ce.Agent
	if agent == "" {
		agent = agentDir
	}
	project := ce.Project
	if project == "" {
		project = resolve("")
	}
	branch := ce.Branch
	if branch == "HEAD" {
		branch = ""
	}
	ev := Event{
		Time:       t,
		Agent:      agent,
		Project:    project,
		Branch:     branch,
		Model:      ce.Model,
		TokensIn:   ce.Usage.Input + ce.Usage.CacheWrite,
		TokensOut:  ce.Usage.Output,
		CacheRead:  ce.Usage.CacheRead,
		CacheWrite: ce.Usage.CacheWrite,
	}
	if ce.Usage.Cost != nil {
		ev.CostInput = ce.Usage.Cost.Input
		ev.CostOutput = ce.Usage.Cost.Output
		ev.CostTotal = ce.Usage.Cost.Total
	}
	return ev, true
}

func tailCanonicalFile(path string, offset int64, resolve func(cwd string) string) ([]Event, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset, err
	}

	agentDir := filepath.Base(filepath.Dir(path))
	var events []Event
	newOffset := offset
	reader := bufio.NewReaderSize(f, 256*1024)

	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}
		newOffset += int64(len(raw))
		if ev, ok := canonicalEventFrom(raw, agentDir, resolve); ok {
			events = append(events, ev)
		}
	}
	return events, newOffset, nil
}
