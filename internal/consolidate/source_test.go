package consolidate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeEvents(t *testing.T, agent, fileName, content string) string {
	t.Helper()
	dir := t.TempDir()
	agentDir := filepath.Join(dir, agent)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, fileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeAgentEvents(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	agentDir := filepath.Join(dir, "pi")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(agentDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func checkFirstEpisode(t *testing.T, episodes []Episode, wantText, wantRef string) {
	t.Helper()
	if episodes[0].Text != wantText {
		t.Errorf("text = %q, want %q", episodes[0].Text, wantText)
	}
	if episodes[0].Refs[0] != wantRef {
		t.Errorf("ref = %q, want %q", episodes[0].Refs[0], wantRef)
	}
	if episodes[0].Timestamp.IsZero() {
		t.Error("episode timestamp is zero")
	}
}

type eventsCase struct {
	name      string
	files     map[string]string
	wantCount int
	wantText  string
	wantRef   string
}

func eventsSourceCases(ts1, ts2 string) []eventsCase {
	return []eventsCase{
		{
			name:      "pi schema yields message text",
			files:     map[string]string{"pi.jsonl": "{\"timestamp\":\"" + ts1 + "\",\"message\":{\"content\":\"the fix was to bump the timeout\"}}\n"},
			wantCount: 1,
			wantText:  "the fix was to bump the timeout",
			wantRef:   "pi/pi.jsonl:1",
		},
		{
			name: "foreign schema still yields text",
			files: map[string]string{
				"weird.jsonl": "{\"created_at\":\"" + ts2 + "\",\"payload\":{\"text\":\"turns out cache keys collide\",\"other\":\"ignored\"}}\n" +
					"{\"no_time\":true,\"message\":\"skipped without timestamp\"}\n" +
					"not json at all\n",
			},
			wantCount: 1,
			wantText:  "turns out cache keys collide",
			wantRef:   "pi/weird.jsonl:1",
		},
		{
			name: "multiple files merge sorted by timestamp",
			files: map[string]string{
				"b.jsonl": "{\"timestamp\":\"" + ts2 + "\",\"text\":\"second\"}\n",
				"a.jsonl": "{\"timestamp\":\"" + ts1 + "\",\"text\":\"first\"}\n",
			},
			wantCount: 2,
			wantText:  "first",
			wantRef:   "pi/a.jsonl:1",
		},
		{
			name:      "empty file yields nothing",
			files:     map[string]string{"empty.jsonl": ""},
			wantCount: 0,
		},
	}
}

func TestEventsSourceSince(t *testing.T) {
	watermark := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	tests := eventsSourceCases("2026-08-20T10:00:00Z", "2026-08-21T12:30:00.500Z")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAgentEvents(t, dir, tt.files)
			src := &EventsSource{Agent: "pi", EventsDir: dir}
			episodes, err := src.Since(watermark)
			if err != nil {
				t.Fatal(err)
			}
			if len(episodes) != tt.wantCount {
				t.Fatalf("got %d episodes, want %d", len(episodes), tt.wantCount)
			}
			if tt.wantCount == 0 {
				return
			}
			checkFirstEpisode(t, episodes, tt.wantText, tt.wantRef)
		})
	}
}

func TestEventsSourceWatermarkFilters(t *testing.T) {
	content := "{\"timestamp\":\"2026-08-20T10:00:00Z\",\"text\":\"before\"}\n" +
		"{\"timestamp\":\"2026-08-22T10:00:00Z\",\"text\":\"after\"}\n"
	dir := writeEvents(t, "pi", "events.jsonl", content)
	src := &EventsSource{Agent: "pi", EventsDir: dir}
	episodes, err := src.Since(time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].Text != "after" {
		t.Fatalf("want only 'after', got %+v", episodes)
	}
}

func TestEventsSourceMissingDir(t *testing.T) {
	src := &EventsSource{Agent: "ghost", EventsDir: t.TempDir()}
	episodes, err := src.Since(time.Time{})
	if err != nil || len(episodes) != 0 {
		t.Fatalf("missing dir should yield empty, got %v, %v", episodes, err)
	}
}

// TestExtractTextIsDeterministic pins the sorted-key walk. A record carrying
// two of message/content/text at one level used to assemble in Go's randomised
// map order, so the same line produced a different Episode.Text run to run —
// and the cursor hash, the similarity score and the NOOP-or-CREATE decision all
// read that text.
func TestExtractTextIsDeterministic(t *testing.T) {
	line := `{"ts":"2026-08-24T10:00:00Z","message":"alpha","content":"bravo","text":"charlie",` +
		`"nested":{"text":"delta","content":"echo"}}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(line), &doc); err != nil {
		t.Fatal(err)
	}
	first := extractText(doc)
	if strings.Count(first, "\n") != 4 {
		t.Fatalf("every text-bearing value must survive: %q", first)
	}
	for i := 0; i < 200; i++ {
		var again map[string]any
		if err := json.Unmarshal([]byte(line), &again); err != nil {
			t.Fatal(err)
		}
		if got := extractText(again); got != first {
			t.Fatalf("extractText is not stable:\n%q\n%q", first, got)
		}
	}
	if want := "bravo\nalpha\necho\ndelta\ncharlie"; first != want {
		t.Fatalf("keys must be visited in sorted order, got %q want %q", first, want)
	}
}
