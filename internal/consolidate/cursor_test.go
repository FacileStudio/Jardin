package consolidate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCursorMissingFile(t *testing.T) {
	c, err := LoadCursor(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 0 {
		t.Fatalf("expected empty cursor, got %v", c.Sources)
	}
}

func TestCursorMarkAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadCursor(dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	hash := HashLine(`{"text":"x"}`)
	c.MarkProcessed("pi", ts, hash)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCursor(dir)
	if err != nil {
		t.Fatal(err)
	}
	pos, ok := loaded.PositionFor("pi")
	if !ok || !pos.Timestamp.Equal(ts) || pos.LastHash != hash {
		t.Fatalf("round trip mismatch: %+v ok=%v", pos, ok)
	}
}

func TestCursorNeverRewinds(t *testing.T) {
	c := &Cursor{Sources: map[string]Position{}}
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newer := old.Add(48 * time.Hour)
	c.MarkProcessed("pi", newer, "b")
	c.MarkProcessed("pi", old, "a")
	pos, _ := c.PositionFor("pi")
	if !pos.Timestamp.Equal(newer) || pos.LastHash != "b" {
		t.Fatalf("stale mark rewound cursor: %+v", pos)
	}
}

func TestCursorSaveAtomicShape(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadCursor(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.MarkProcessed("pi", time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), "abc")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".consolidate-cursor.json")); err != nil {
		t.Fatalf("cursor file missing after save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".consolidate-cursor.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not survive save: %v", err)
	}
}

func TestInterruptedRunReprocessesNothing(t *testing.T) {
	line := `{"timestamp":"2026-08-23T08:00:00Z","text":"note that the flag is sticky"}`
	dir := writeEvents(t, "pi", "events.jsonl", line+"\n")
	src := &EventsSource{Agent: "pi", EventsDir: dir}

	runOnce := func(c *Cursor) int {
		episodes, err := src.Since(cursorTime(c, "pi"))
		if err != nil {
			t.Fatal(err)
		}
		for _, ep := range episodes {
			c.MarkProcessed("pi", ep.Timestamp, HashLine(line))
		}
		if err := c.Save(); err != nil {
			t.Fatal(err)
		}
		return len(episodes)
	}

	first, err := LoadCursor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := runOnce(first); n != 1 {
		t.Fatalf("first run processed %d, want 1", n)
	}
	second, err := LoadCursor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := runOnce(second); n != 0 {
		t.Fatalf("second run reprocessed %d episodes, want 0", n)
	}
}

func cursorTime(c *Cursor, source string) time.Time {
	if pos, ok := c.PositionFor(source); ok {
		return pos.Timestamp
	}
	return time.Time{}
}
