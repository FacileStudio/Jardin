package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectCanonicalParsesEvents(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events", "pi")
	os.MkdirAll(eventsDir, 0755)

	content := "{\"type\":\"message\",\"role\":\"assistant\",\"timestamp\":\"2026-08-11T11:49:33.315Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\",\"branch\":\"main\",\"model\":\"deepseek/deepseek-v4-flash\",\"usage\":{\"input\":1259,\"output\":54,\"cacheRead\":256,\"cacheWrite\":0,\"totalTokens\":1569}}\n" +
		"{\"type\":\"message\",\"role\":\"assistant\",\"timestamp\":\"2026-08-11T11:50:00.000Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\",\"branch\":\"main\",\"model\":\"deepseek/deepseek-v4-flash\",\"usage\":{\"input\":500,\"output\":100,\"cacheRead\":0,\"cacheWrite\":0,\"totalTokens\":600}}\n"

	os.WriteFile(filepath.Join(eventsDir, "2026-08.jsonl"), []byte(content), 0644)

	state := newScanState()
	resolve := func(cwd string) string { return "Jardin" }

	events, err := collectCanonical(dir, state, resolve)
	if err != nil {
		t.Fatalf("collectCanonical: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].TokensIn != 1259 || events[0].TokensOut != 54 {
		t.Fatalf("bad tokens: in=%d out=%d", events[0].TokensIn, events[0].TokensOut)
	}
	if events[0].Agent != "pi" || events[0].Project != "Jardin" {
		t.Fatalf("bad metadata: agent=%s project=%s", events[0].Agent, events[0].Project)
	}
}

func TestCollectCanonicalSkipsUserMessages(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events", "pi")
	os.MkdirAll(eventsDir, 0755)

	content := "{\"type\":\"message\",\"role\":\"user\",\"timestamp\":\"2026-08-11T11:49:00.000Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\"}\n" +
		"{\"type\":\"message\",\"role\":\"assistant\",\"timestamp\":\"2026-08-11T11:49:33.315Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\",\"usage\":{\"input\":100,\"output\":50,\"cacheRead\":0,\"cacheWrite\":0,\"totalTokens\":150}}\n"

	os.WriteFile(filepath.Join(eventsDir, "2026-08.jsonl"), []byte(content), 0644)

	state := newScanState()
	resolve := func(cwd string) string { return "Jardin" }

	events, err := collectCanonical(dir, state, resolve)
	if err != nil {
		t.Fatalf("collectCanonical: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (assistant only), got %d", len(events))
	}
}

func TestCollectCanonicalResumesFromOffset(t *testing.T) {
	dir := t.TempDir()
	eventsDir := filepath.Join(dir, "events", "pi")
	os.MkdirAll(eventsDir, 0755)
	path := filepath.Join(eventsDir, "2026-08.jsonl")

	content := "{\"type\":\"message\",\"role\":\"assistant\",\"timestamp\":\"2026-08-11T11:49:33.315Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\",\"usage\":{\"input\":100,\"output\":50,\"cacheRead\":0,\"cacheWrite\":0,\"totalTokens\":150}}\n"
	os.WriteFile(path, []byte(content), 0644)

	state := newScanState()
	resolve := func(cwd string) string { return "Jardin" }

	events, err := collectCanonical(dir, state, resolve)
	if err != nil || len(events) != 1 {
		t.Fatalf("first read: events=%d err=%v", len(events), err)
	}

	events, err = collectCanonical(dir, state, resolve)
	if err != nil || len(events) != 0 {
		t.Fatalf("resume: expected 0 events, got %d (err=%v)", len(events), err)
	}

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("{\"type\":\"message\",\"role\":\"assistant\",\"timestamp\":\"2026-08-11T11:50:00.000Z\",\"agent\":\"pi\",\"machine\":\"lucy\",\"project\":\"Jardin\",\"usage\":{\"input\":200,\"output\":75,\"cacheRead\":0,\"cacheWrite\":0,\"totalTokens\":275}}\n")
	f.Close()

	events, err = collectCanonical(dir, state, resolve)
	if err != nil || len(events) != 1 {
		t.Fatalf("append: expected 1 event, got %d (err=%v)", len(events), err)
	}
	if events[0].TokensIn != 200 || events[0].TokensOut != 75 {
		t.Fatalf("bad tokens: in=%d out=%d", events[0].TokensIn, events[0].TokensOut)
	}
}
