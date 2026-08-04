package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

func ev(offset time.Duration, project string, out int64) Event {
	return Event{Time: t0.Add(offset), Agent: "claude", Project: project, TokensOut: out}
}

func TestFoldMergesWithinGap(t *testing.T) {
	state := newScanState()
	sealed := fold(state, "lucy", []Event{
		ev(0, "Mycelium", 100),
		ev(5*time.Minute, "Mycelium", 50),
		ev(14*time.Minute, "Mycelium", 25),
	}, t0.Add(20*time.Minute))

	if len(sealed) != 0 {
		t.Fatalf("expected no sealed blocks, got %d", len(sealed))
	}
	open := state.Open["mycelium|claude"]
	if open == nil {
		t.Fatal("expected open block")
	}
	if open.Events != 3 || open.TokensOut != 175 {
		t.Fatalf("bad accumulation: events=%d tokens=%d", open.Events, open.TokensOut)
	}
	if open.Duration() != 14*time.Minute {
		t.Fatalf("bad duration: %s", open.Duration())
	}
}

func TestFoldSealsOnGap(t *testing.T) {
	state := newScanState()
	sealed := fold(state, "lucy", []Event{
		ev(0, "Mycelium", 100),
		ev(16*time.Minute, "Mycelium", 50),
	}, t0.Add(17*time.Minute))

	if len(sealed) != 1 {
		t.Fatalf("expected 1 sealed block, got %d", len(sealed))
	}
	if sealed[0].Duration() != 0 {
		t.Fatalf("isolated heartbeat must have zero duration, got %s", sealed[0].Duration())
	}
	if sealed[0].ID == "" {
		t.Fatal("sealed block must carry an id")
	}
}

func TestFoldSealsStaleOpenBlocks(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{ev(0, "Mycelium", 100)}, t0.Add(1*time.Minute))
	sealed := fold(state, "lucy", nil, t0.Add(31*time.Minute))

	if len(sealed) != 1 {
		t.Fatalf("expected stale block sealed, got %d", len(sealed))
	}
	if len(state.Open) != 0 {
		t.Fatalf("expected no open blocks, got %d", len(state.Open))
	}
}

func TestFoldSeparatesProjects(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{
		ev(0, "Mycelium", 10),
		ev(time.Minute, "Sablier", 20),
	}, t0.Add(2*time.Minute))

	if len(state.Open) != 2 {
		t.Fatalf("expected 2 open blocks, got %d", len(state.Open))
	}
}

func TestFoldMergesCaseVariants(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{
		{Time: t0, Agent: "claude", Project: "GFConseil", TokensOut: 10},
		{Time: t0.Add(time.Minute), Agent: "claude", Project: "gfconseil", TokensOut: 20},
	}, t0.Add(2*time.Minute))

	if len(state.Open) != 1 {
		t.Fatalf("case variants must merge into one block, got %d", len(state.Open))
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:FacileStudio/GFConseil.git": "GFConseil",
		"https://github.com/FacileStudio/Mycelium":    "Mycelium",
		"https://github.com/owner/repo.name.git":    "repo.name",
		"ssh://git@host.com/owner/Repo/":            "Repo",
		"":                                          "",
		"..":                                        "",
	}
	for in, want := range cases {
		if got := repoNameFromRemote(in); got != want {
			t.Fatalf("repoNameFromRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAggregateFoldsCase(t *testing.T) {
	blocks := []Block{
		{Project: "GFConseil", StartedAt: t0, EndedAt: t0.Add(time.Hour), TokensOut: 10},
		{Project: "gfconseil", StartedAt: t0, EndedAt: t0.Add(time.Hour), TokensOut: 20},
	}
	rows := Aggregate(blocks, time.Time{}, "project")
	if len(rows) != 1 {
		t.Fatalf("case variants must aggregate together, got %d rows", len(rows))
	}
	if rows[0].Key != "GFConseil" || rows[0].TokensOut != 30 {
		t.Fatalf("bad folded row: %+v", rows[0])
	}
}

func TestBlockIDDeterministic(t *testing.T) {
	a := Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0}
	b := Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0}
	if a.computeID() != b.computeID() {
		t.Fatal("same natural key must yield same id")
	}
	c := Block{Project: "Mycelium", Machine: "ruche", Agent: "claude", StartedAt: t0}
	if a.computeID() == c.computeID() {
		t.Fatal("different machine must yield different id")
	}
}

func writeTranscript(t *testing.T, dir, session string, lines []string) string {
	t.Helper()
	projDir := filepath.Join(dir, "projects", "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, session+".jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assistantLine(ts, reqID string, out int64) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":"/Users/test/proj","gitBranch":"main","requestId":%q,"message":{"model":"claude-opus-5","usage":{"input_tokens":2,"output_tokens":%d,"cache_creation_input_tokens":10,"cache_read_input_tokens":100}}}`, ts, reqID, out)
}

func TestCollectClaudeDedupsRequestIDs(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, "s1", []string{
		`{"type":"user","timestamp":"2026-08-01T10:00:00.000Z","cwd":"/Users/test/proj","gitBranch":"main"}`,
		assistantLine("2026-08-01T10:00:05.000Z", "req_1", 300),
		assistantLine("2026-08-01T10:00:06.000Z", "req_1", 300),
		assistantLine("2026-08-01T10:00:07.000Z", "req_1", 300),
		assistantLine("2026-08-01T10:00:20.000Z", "req_2", 50),
		`{"type":"ai-title","sessionId":"x"}`,
	})

	state := newScanState()
	events, err := collectClaude(dir, state, func(string) string { return "proj" })
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range events {
		total += e.TokensOut
	}
	if total != 350 {
		t.Fatalf("expected 350 output tokens after dedup, got %d", total)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 heartbeat events, got %d", len(events))
	}
}

func TestCollectClaudeResumesFromOffset(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s1", []string{
		assistantLine("2026-08-01T10:00:00.000Z", "req_1", 100),
	})

	state := newScanState()
	first, err := collectClaude(dir, state, func(string) string { return "proj" })
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 event, got %d", len(first))
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(assistantLine("2026-08-01T10:01:00.000Z", "req_1", 100) + "\n")
	f.WriteString(assistantLine("2026-08-01T10:02:00.000Z", "req_2", 40) + "\n")
	f.Close()

	second, err := collectClaude(dir, state, func(string) string { return "proj" })
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range second {
		total += e.TokensOut
	}
	if total != 40 {
		t.Fatalf("requestId straddling scans must not double-count: got %d", total)
	}
}

func TestCollectClaudeIgnoresPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscript(t, dir, "s1", []string{
		assistantLine("2026-08-01T10:00:00.000Z", "req_1", 100),
	})
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"type":"user","timestamp":"2026-08-01T10:00:30.000Z","cwd":"/Users/te`)
	f.Close()

	state := newScanState()
	events, err := collectClaude(dir, state, func(string) string { return "proj" })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("partial trailing line must be ignored, got %d events", len(events))
	}

	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("st/proj\"}\n")
	f.Close()
	more, err := collectClaude(dir, state, func(string) string { return "proj" })
	if err != nil {
		t.Fatal(err)
	}
	if len(more) != 1 {
		t.Fatalf("completed line must be picked up on next scan, got %d", len(more))
	}
}

func TestReadBlocksDedupesById(t *testing.T) {
	dir := t.TempDir()
	block := finalize(&Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0, EndedAt: t0.Add(10 * time.Minute)})
	if err := appendBlocks(dir, "lucy", []Block{block, block}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("duplicate ids must collapse to one block, got %d", len(got))
	}
}

func TestScanLockExcludesConcurrentScan(t *testing.T) {
	dir := t.TempDir()
	release, err := lockScan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(dir, "lucy", filepath.Join(dir, "no-claude"), t0); err == nil {
		t.Fatal("scan must refuse while lock is held")
	}
	release()
	if _, err := Scan(dir, "lucy", filepath.Join(dir, "no-claude"), t0); err != nil {
		t.Fatalf("scan must succeed after release: %v", err)
	}
}

func TestShardRoundTrip(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		finalize(&Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0, EndedAt: t0.Add(10 * time.Minute), Events: 3, TokensOut: 500}),
		finalize(&Block{Project: "Sablier", Machine: "lucy", Agent: "claude", StartedAt: t0.Add(time.Hour), EndedAt: t0.Add(time.Hour + 5*time.Minute), Events: 2}),
	}
	if err := appendBlocks(dir, "lucy", blocks); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got))
	}
	if got[0].Project != "Mycelium" || got[1].Project != "Sablier" {
		t.Fatalf("blocks out of order: %s, %s", got[0].Project, got[1].Project)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	state := newScanState()
	state.Files["/x.jsonl"] = &FileState{Offset: 42, Size: 100, RecentReqs: []string{"req_1"}}
	state.Projects["/Users/test/proj"] = "proj"
	fold(state, "lucy", []Event{ev(0, "Mycelium", 10)}, t0.Add(time.Minute))
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	loaded := LoadState(dir)
	if loaded.Files["/x.jsonl"].Offset != 42 {
		t.Fatal("file state lost")
	}
	if loaded.Open["mycelium|claude"] == nil {
		t.Fatal("open block lost")
	}
	data, _ := json.Marshal(loaded.Open["mycelium|claude"])
	if len(data) == 0 {
		t.Fatal("open block must marshal")
	}
}

func TestAggregate(t *testing.T) {
	blocks := []Block{
		{Project: "Mycelium", Machine: "lucy", StartedAt: t0, EndedAt: t0.Add(30 * time.Minute), TokensIn: 10, CacheWrite: 90, TokensOut: 100},
		{Project: "Mycelium", Machine: "ruche", StartedAt: t0.Add(time.Hour), EndedAt: t0.Add(time.Hour + 30*time.Minute), TokensOut: 50},
		{Project: "Old", Machine: "lucy", StartedAt: t0.Add(-60 * 24 * time.Hour), EndedAt: t0.Add(-60 * 24 * time.Hour).Add(time.Hour)},
	}
	rows := Aggregate(blocks, t0.Add(-24*time.Hour), "project")
	if len(rows) != 1 {
		t.Fatalf("since filter failed: %d rows", len(rows))
	}
	if rows[0].Sessions != 2 || rows[0].Seconds != 3600 {
		t.Fatalf("bad row: %+v", rows[0])
	}
	if rows[0].TokensIn != 100 {
		t.Fatalf("tokens_in must include cache_write: %d", rows[0].TokensIn)
	}

	byMachine := Aggregate(blocks, time.Time{}, "machine")
	if len(byMachine) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(byMachine))
	}
}

func TestRecap(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{finalize(&Block{
		Project: "Mycelium", Machine: "lucy", Agent: "claude", Branch: "main",
		StartedAt: t0, EndedAt: t0.Add(30 * time.Minute), Events: 5, TokensOut: 12000,
	})}
	if err := appendBlocks(dir, "lucy", blocks); err != nil {
		t.Fatal(err)
	}
	recap := Recap(dir, "Mycelium", t0.Add(2*time.Hour))
	if recap == "" {
		t.Fatal("expected recap")
	}
	for _, want := range []string{"Mycelium", "lucy", "branch main", "12.0k"} {
		if !contains(recap, want) {
			t.Fatalf("recap missing %q:\n%s", want, recap)
		}
	}
	if Recap(dir, "Unknown", t0) != "" {
		t.Fatal("unknown project must produce empty recap")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
