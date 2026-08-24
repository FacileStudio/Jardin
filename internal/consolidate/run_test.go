package consolidate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var runTestNow = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

func seedRunEvents(t *testing.T, dataDir, agent, content string) string {
	t.Helper()
	dir := filepath.Join(dataDir, "events", agent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "2026-08.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	return path
}

const gotchaLine = `{"timestamp":"2026-08-25T10:00:00Z","content":"note that the installer always pins the versioned cellar path on upgrades"}`

func findPage(t *testing.T, dataDir string) string {
	t.Helper()
	var found string
	filepath.Walk(filepath.Join(dataDir, "memory"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			found = path
		}
		return nil
	})
	return found
}

func TestRunWritesFindingAndAdvancesCursor(t *testing.T) {
	dataDir := t.TempDir()
	seedRunEvents(t, dataDir, "pi", gotchaLine+"\n")

	res, err := Run(dataDir, Options{Force: true, Now: runTestNow})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Skipped != "" {
		t.Fatalf("expected one create, got %+v", res)
	}
	page := findPage(t, dataDir)
	if page == "" {
		t.Fatal("no memory page written")
	}
	data, _ := os.ReadFile(page)
	if !strings.Contains(string(data), "**Date**: 2026-08-26") ||
		!strings.Contains(string(data), "### ") {
		t.Fatalf("page missing convention block:\n%s", data)
	}

	cursor, err := LoadCursor(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	pos, ok := cursor.PositionFor("pi")
	if !ok || pos.Timestamp.IsZero() {
		t.Fatalf("cursor not advanced: %+v", cursor.Sources)
	}

	state, err := LoadState(dataDir)
	if err != nil || state == nil {
		t.Fatalf("state not saved: %v %v", state, err)
	}
	if !state.LastRun.Equal(runTestNow) || state.Result == nil || state.Result.Created != 1 {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestRunSkipsWhenNoEventsChangedSinceWatermark(t *testing.T) {
	dataDir := t.TempDir()
	path := seedRunEvents(t, dataDir, "pi", gotchaLine+"\n")

	if _, err := Run(dataDir, Options{Force: true, Now: runTestNow}); err != nil {
		t.Fatal(err)
	}
	before := findPage(t, dataDir)

	future := runTestNow.Add(2 * time.Hour)
	res, err := Run(dataDir, Options{Force: true, Now: future})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped == "" || res.Created != 0 {
		t.Fatalf("expected a change-free skip, got %+v", res)
	}
	if after := findPage(t, dataDir); after != before {
		t.Fatal("skip must write nothing")
	}

	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	res, err = Run(dataDir, Options{Force: true, Now: future.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != "" {
		t.Fatalf("a touched events file must re-run, got skipped=%q", res.Skipped)
	}
}

func TestRunRateLimitsWithoutForce(t *testing.T) {
	dataDir := t.TempDir()
	path := seedRunEvents(t, dataDir, "pi", gotchaLine+"\n")

	first, err := Run(dataDir, Options{Now: runTestNow})
	if err != nil || first.Created != 1 {
		t.Fatalf("first run should consolidate: %+v %v", first, err)
	}
	second, err := Run(dataDir, Options{Now: runTestNow.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Skipped == "" || second.Created != 0 {
		t.Fatalf("expected rate-limit skip, got %+v", second)
	}
	future := runTestNow.Add(30 * time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	forced, err := Run(dataDir, Options{Force: true, Now: future})
	if err != nil {
		t.Fatal(err)
	}
	if forced.Skipped != "" {
		t.Fatalf("force must bypass the rate limit, got skipped=%q", forced.Skipped)
	}
}

func TestRunDropsSecretCandidatesWithReason(t *testing.T) {
	dataDir := t.TempDir()
	secret := `{"timestamp":"2026-08-25T11:00:00Z","content":"note that the deploy script always writes API_KEY=supersecretvalue123 into the compose file"}`
	seedRunEvents(t, dataDir, "pi", secret+"\n")

	res, err := Run(dataDir, Options{Force: true, Now: runTestNow})
	if err != nil {
		t.Fatal(err)
	}
	if res.Dropped == 0 || len(res.Reasons) == 0 {
		t.Fatalf("expected a drop with reason, got %+v", res)
	}
	joined := strings.Join(res.Reasons, "\n")
	if !strings.Contains(joined, RuleSecret) && !strings.Contains(joined, "judge refused") {
		t.Fatalf("reasons lack rule name or judge verdict: %q", joined)
	}
	if findPage(t, dataDir) != "" {
		t.Fatal("a dropped candidate must never reach memory/")
	}
}

func TestRunContainsCandidateWriteFailuresAndAdvancesCursors(t *testing.T) {
	dataDir := t.TempDir()
	seedRunEvents(t, dataDir, "claude", gotchaLine+"\n")
	seedRunEvents(t, dataDir, "pi", gotchaLine+"\n")
	memoryPath := filepath.Join(dataDir, "memory")
	if err := os.WriteFile(memoryPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Run(dataDir, Options{Force: true, Now: runTestNow})
	if err != nil {
		t.Fatalf("a candidate-level failure must be contained, got %v", err)
	}
	if res.Created != 0 || res.Dropped != 2 || len(res.Reasons) != 2 {
		t.Fatalf("expected one drop per candidate with a reason each, got %+v", res)
	}
	for _, r := range res.Reasons {
		if !strings.Contains(r, "candidate failed") {
			t.Fatalf("drop reason lacks failure detail: %q", r)
		}
	}
	if findPage(t, dataDir) != "" {
		t.Fatal("failed candidates must never reach memory/")
	}
	cursor, err := LoadCursor(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude", "pi"} {
		if pos, ok := cursor.PositionFor(agent); !ok || pos.Timestamp.IsZero() {
			t.Fatalf("cursor not advanced for %s: %+v", agent, cursor.Sources)
		}
	}
	state, err := LoadState(dataDir)
	if err != nil || state == nil || state.Error != "" || state.Result == nil {
		t.Fatalf("contained failures are not a failed run, state: %+v %v", state, err)
	}
}

func TestRunSkipsUnreadableAgentButConsolidatesOthers(t *testing.T) {
	dataDir := t.TempDir()
	blocked := seedRunEvents(t, dataDir, "aaa", gotchaLine+"\n")
	seedRunEvents(t, dataDir, "zzz", gotchaLine+"\n")
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Skipf("cannot make the file unreadable here: %v", err)
	}
	defer os.Chmod(blocked, 0o600)

	res, err := Run(dataDir, Options{Force: true, Now: runTestNow})
	if err != nil {
		t.Fatalf("one broken agent must not fail the run, got %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("the healthy agent must still consolidate, got %+v", res)
	}
	cursor, err := LoadCursor(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if pos, ok := cursor.PositionFor("zzz"); !ok || pos.Timestamp.IsZero() {
		t.Fatalf("healthy agent cursor not advanced: %+v", cursor.Sources)
	}
	if _, ok := cursor.PositionFor("aaa"); ok {
		t.Fatal("skipped agent must keep its watermark untouched")
	}
}

func TestApplyCountsOneDropPerCandidateWithAllRejectionReasons(t *testing.T) {
	p := newPipeline(t.TempDir(), "")
	p.now = runTestNow
	res := Result{}
	p.apply(Candidate{Text: "just a note here", EpisodeRefs: []string{"pi/2026-08.jsonl:1"}}, &res)
	if res.Dropped != 1 {
		t.Fatalf("a twice-rejected candidate is still exactly one drop: %+v", res)
	}
	joined := strings.Join(res.Reasons, "\n")
	if !strings.Contains(joined, RuleBehavior) || !strings.Contains(joined, RuleObvious) {
		t.Fatalf("every rejection reason must survive aggregation: %q", joined)
	}
}

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

func TestMain(m *testing.M) {
	os.Setenv("OLLAMA_URL", "http://127.0.0.1:1")
	os.Exit(m.Run())
}
