package consolidate

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
)

// Episode is one extracted unit of episodic memory: what happened, when,
// and where it came from.
type Episode struct {
	Agent     string
	Timestamp time.Time
	Text      string
	Refs      []string
}

// Source produces episodes for one harness or feed. Implementations key on
// directory layout, never on a specific event schema.
type Source interface {
	Name() string
	Since(watermark time.Time) ([]Episode, error)
}

// EventsDir is where raw episode JSONL files land, one subdirectory per agent.
func EventsDir() string { return filepath.Join(config.DataDir(), "events") }

// EventsSource reads events/<agent>/*.jsonl under the mycelium data dir. The
// reader knows nothing about any harness's schema: text is pulled best-effort
// from any "message", "content" or "text" string value in each line, and the
// timestamp from any "timestamp", "time" or "created_at" field. Lines without
// a parsable timestamp are skipped — an episode with no time cannot be
// watermarked, so including it would break idempotency.
type EventsSource struct {
	Agent     string
	EventsDir string
}

// NewEventsSource returns an EventsSource reading the given agent's events
// from the default data dir.
func NewEventsSource(agent string) *EventsSource {
	return &EventsSource{Agent: agent, EventsDir: EventsDir()}
}

func (s *EventsSource) Name() string { return s.Agent }

func (s *EventsSource) Since(watermark time.Time) ([]Episode, error) {
	dir := filepath.Join(s.EventsDir, s.Agent)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var episodes []Episode
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fileEpisodes, err := readJSONL(path, s.Agent, entry.Name(), watermark)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, fileEpisodes...)
	}
	sort.Slice(episodes, func(i, j int) bool {
		return episodes[i].Timestamp.Before(episodes[j].Timestamp)
	})
	return episodes, nil
}

func readJSONL(path, agent, fileName string, watermark time.Time) ([]Episode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var episodes []Episode
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var doc map[string]any
		if json.Unmarshal([]byte(line), &doc) != nil {
			continue
		}
		ts, ok := extractTimestamp(doc)
		if !ok || !ts.After(watermark) {
			continue
		}
		text := strings.TrimSpace(extractText(doc))
		if text == "" {
			continue
		}
		episodes = append(episodes, Episode{
			Agent:     agent,
			Timestamp: ts,
			Text:      text,
			Refs:      []string{filepath.ToSlash(filepath.Join(agent, fileName)) + ":" + itoa(lineNo)},
		})
	}
	return episodes, scanner.Err()
}

var textKeys = map[string]bool{"message": true, "content": true, "text": true}
var timeKeys = map[string]bool{"timestamp": true, "time": true, "created_at": true}

func extractText(doc map[string]any) string {
	var b strings.Builder
	collectText(doc, &b)
	return b.String()
}

func collectText(node any, b *strings.Builder) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			collectKeyedText(key, child, b)
		}
	case []any:
		for _, child := range v {
			collectText(child, b)
		}
	}
}

func collectKeyedText(key string, child any, b *strings.Builder) {
	if textKeys[strings.ToLower(key)] {
		if s, ok := child.(string); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(s)
			return
		}
	}
	collectText(child, b)
}

func extractTimestamp(doc map[string]any) (time.Time, bool) {
	for _, key := range []string{"timestamp", "time", "created_at"} {
		raw, ok := doc[key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if ts, err := time.Parse(layout, s); err == nil {
				return ts, true
			}
		}
	}
	return time.Time{}, false
}

func itoa(n int) string { return strconv.Itoa(n) }

// LocalEventsDir is where harnesses log episode text that stays on this
// machine: syncSkip excludes it, so raw conversations never reach the server.
func LocalEventsDir(dataDir string) string {
	return filepath.Join(dataDir, "local", "events")
}

// eventSources lists the agent directories under the events dir, sorted for
// deterministic runs; a missing events dir means nothing to do.
func eventSources(eventsDir string) ([]string, error) {
	entries, err := os.ReadDir(eventsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, e := range entries {
		if e.IsDir() {
			agents = append(agents, e.Name())
		}
	}
	sort.Strings(agents)
	return agents, nil
}

// mergedSourceNames lists agents present in either the synced events dir or
// the local-only dir, sorted and deduplicated.
func mergedSourceNames(eventsDir, dataDir string) ([]string, error) {
	synced, err := eventSources(eventsDir)
	if err != nil {
		return nil, err
	}
	local, err := eventSources(LocalEventsDir(dataDir))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(synced)+len(local))
	for _, a := range append(synced, local...) {
		seen[a] = true
	}
	names := make([]string, 0, len(seen))
	for a := range seen {
		names = append(names, a)
	}
	sort.Strings(names)
	return names, nil
}

// mergedEpisodes reads one agent's episodes from both the synced events dir
// and the local-only dir (where harnesses log message text that must never
// sync), sorted by timestamp.
func mergedEpisodes(agent, eventsDir, dataDir string, watermark time.Time) ([]Episode, error) {
	synced, err := (&EventsSource{Agent: agent, EventsDir: eventsDir}).Since(watermark)
	if err != nil {
		return nil, err
	}
	local, err := (&EventsSource{Agent: agent, EventsDir: LocalEventsDir(dataDir)}).Since(watermark)
	if err != nil {
		return nil, err
	}
	episodes := append(synced, local...)
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].Timestamp.Before(episodes[j].Timestamp) })
	return episodes, nil
}
