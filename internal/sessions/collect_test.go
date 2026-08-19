package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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
