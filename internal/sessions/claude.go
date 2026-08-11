package sessions

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const recentReqCap = 64

type modelPricing struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

var claudePricing = map[string]modelPricing{
	"claude-sonnet-4-20250514":   {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-sonnet-4-5-20250929": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-3-5-sonnet-20241022": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-3-5-haiku-20241022":  {Input: 0.80, Output: 4.00, CacheRead: 0.08, CacheWrite: 1.00},
	"claude-3-opus-20240229":     {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
	"claude-opus-4-20250514":     {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
	"claude-opus-4-5-20251101":   {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
}

var claudeFamilyPricing = map[string]modelPricing{
	"claude-sonnet": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-opus":   {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
	"claude-haiku":  {Input: 0.80, Output: 4.00, CacheRead: 0.08, CacheWrite: 1.00},
	"claude-fable":  {Input: 0.80, Output: 4.00, CacheRead: 0.08, CacheWrite: 1.00},
}

func matchPricing(model string) (modelPricing, bool) {
	if p, ok := claudePricing[model]; ok {
		return p, true
	}
	lower := strings.ToLower(model)
	for prefix, p := range claudeFamilyPricing {
		if strings.HasPrefix(lower, prefix) {
			return p, true
		}
	}
	return modelPricing{}, false
}

type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	GitBranch string `json:"gitBranch"`
	RequestID string `json:"requestId"`
	Message   *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// collectClaude tails every Claude Code transcript under claudeDir/projects,
// resuming from per-file byte offsets kept in state. Streamed responses repeat
// identical usage objects across lines sharing a requestId, so tokens are
// counted once per requestId; the recent-id window persists in state to
// survive a scan boundary splitting one request's lines.
func collectClaude(claudeDir string, state *ScanState, resolve func(cwd string) string) ([]Event, error) {
	pattern := filepath.Join(claudeDir, "projects", "*", "*.jsonl")
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
			fs.RecentReqs = nil
		}
		evs, newOffset, err := tailFile(path, fs, resolve)
		if err != nil {
			continue
		}
		fs.Offset = newOffset
		fs.Size = info.Size()
		events = append(events, evs...)
	}
	return events, nil
}

func tailFile(path string, fs *FileState, resolve func(cwd string) string) ([]Event, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fs.Offset, err
	}
	defer f.Close()
	if _, err := f.Seek(fs.Offset, io.SeekStart); err != nil {
		return nil, fs.Offset, err
	}

	seen := make(map[string]bool, len(fs.RecentReqs))
	for _, id := range fs.RecentReqs {
		seen[id] = true
	}

	var events []Event
	offset := fs.Offset
	reader := bufio.NewReaderSize(f, 256*1024)
	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}
		offset += int64(len(raw))
		ev, reqID, ok := parseClaudeLine(raw, resolve)
		if !ok {
			continue
		}
		if reqID != "" {
			if seen[reqID] {
				ev.TokensIn, ev.TokensOut, ev.CacheRead, ev.CacheWrite = 0, 0, 0, 0
				ev.CostInput, ev.CostOutput, ev.CostTotal = 0, 0, 0
			} else {
				seen[reqID] = true
				fs.RecentReqs = append(fs.RecentReqs, reqID)
				if len(fs.RecentReqs) > recentReqCap {
					fs.RecentReqs = fs.RecentReqs[len(fs.RecentReqs)-recentReqCap:]
				}
			}
		}
		events = append(events, ev)
	}
	return events, offset, nil
}

func parseClaudeLine(raw []byte, resolve func(cwd string) string) (Event, string, bool) {
	var l claudeLine
	if err := json.Unmarshal(raw, &l); err != nil {
		return Event{}, "", false
	}
	if l.Type != "user" && l.Type != "assistant" {
		return Event{}, "", false
	}
	if l.Cwd == "" {
		return Event{}, "", false
	}
	t, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return Event{}, "", false
	}

	branch := l.GitBranch
	if branch == "HEAD" {
		branch = ""
	}
	ev := Event{
		Time:    t,
		Agent:   "claude",
		Project: resolve(l.Cwd),
		Branch:  branch,
	}
	reqID := ""
	if l.Message != nil {
		ev.Model = l.Message.Model
		if u := l.Message.Usage; u != nil {
			ev.TokensIn = u.InputTokens
			ev.TokensOut = u.OutputTokens
			ev.CacheRead = u.CacheReadTokens
			ev.CacheWrite = u.CacheCreationTokens
			if pricing, ok := matchPricing(ev.Model); ok {
				ev.CostInput = float64(u.InputTokens) / 1_000_000 * pricing.Input
				ev.CostOutput = float64(u.OutputTokens) / 1_000_000 * pricing.Output
				ev.CostTotal = ev.CostInput + ev.CostOutput
			}
			reqID = l.RequestID
		}
	}
	return ev, reqID, true
}
