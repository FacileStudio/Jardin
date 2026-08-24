package consolidate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadStateAbsentReturnsNilThenRoundTrips(t *testing.T) {
	if s, err := LoadState(t.TempDir()); err != nil || s != nil {
		t.Fatalf("expected nil state, got %v %v", s, err)
	}
	dataDir := t.TempDir()
	want := State{LastRun: runTestNow, Result: &Result{Created: 2, Reasons: []string{RuleObvious + ": x"}}}
	if err := saveState(dataDir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastRun != want.LastRun || got.Result == nil || got.Result.Created != 2 ||
		len(got.Result.Reasons) != 1 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestEarliestWatermarkPicksOldestSource(t *testing.T) {
	c := &Cursor{Sources: map[string]Position{
		"pi": {Timestamp: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)},
		"x":  {Timestamp: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)},
	}}
	wm, ok := earliestWatermark(c)
	if !ok || wm.Equal(c.Sources["pi"].Timestamp) {
		t.Fatalf("expected the oldest watermark, got %v %v", wm, ok)
	}
}

func TestEventSourcesListsSortedDirsAndToleratesAbsence(t *testing.T) {
	if agents, err := eventSources(filepath.Join(t.TempDir(), "nope")); err != nil || agents != nil {
		t.Fatalf("missing events dir must be empty, got %v %v", agents, err)
	}
	dataDir := t.TempDir()
	os.MkdirAll(filepath.Join(dataDir, "events", "pi"), 0o755)
	os.MkdirAll(filepath.Join(dataDir, "events", "claude"), 0o755)
	os.WriteFile(filepath.Join(dataDir, "events", "loose.txt"), nil, 0o600)
	agents, err := eventSources(filepath.Join(dataDir, "events"))
	if err != nil || len(agents) != 2 || agents[0] != "claude" || agents[1] != "pi" {
		t.Fatalf("unexpected sources: %v %v", agents, err)
	}
}

func TestSaveStateJSONShape(t *testing.T) {
	dataDir := t.TempDir()
	saveState(dataDir, State{LastRun: runTestNow, Error: "boom"})
	data, _ := os.ReadFile(StatePath(dataDir))
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["error"] != "boom" || raw["last_run"] == nil {
		t.Fatalf("unexpected JSON shape: %s", data)
	}
}

// TestFailStampsLastRun keeps a broken run off the 60s daemon tick. A skip must
// leave LastRun alone, but a failure has already done the work, and without the
// stamp an unwritable memory dir re-runs the whole pipeline every minute.
func TestFailStampsLastRun(t *testing.T) {
	p := &pipeline{dataDir: t.TempDir(), now: runTestNow}
	if _, err := p.fail(Result{}, errors.New("boom")); err == nil {
		t.Fatal("fail must return the error it recorded")
	}
	state, err := LoadState(p.dataDir)
	if err != nil || state == nil {
		t.Fatalf("state not recorded: %v %v", state, err)
	}
	if !state.LastRun.Equal(runTestNow) || state.Error != "boom" {
		t.Fatalf("want the run stamped and the error kept, got %+v", state)
	}
	if skip := rateLimitSkip(state, Options{Now: runTestNow.Add(time.Minute)}); skip == "" {
		t.Fatal("a failed run must still hold the hourly rate limit")
	}
	if skip := rateLimitSkip(state, Options{Now: runTestNow.Add(time.Minute), Force: true}); skip != "" {
		t.Fatalf("--force must bypass it: %s", skip)
	}
}
