package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
)

func flowBody(name string) string {
	return "name: " + name + "\nsteps:\n  - name: one\n    run: echo hi\n"
}

func writeFlowFile(t *testing.T, name, body string) {
	t.Helper()
	dir := config.FlowsDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+Extension), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func sampleRun(started time.Time) *Run {
	return &Run{
		Flow:         "alpha",
		FlowChecksum: "sha256:deadbeef",
		Machine:      "lucy",
		WorkDir:      "/srv/alpha",
		StartedAt:    started,
		FinishedAt:   started.Add(3 * time.Second),
		Status:       StatusOK,
		Steps: []StepResult{
			{Name: "one", ExitCode: 0, DurationMS: 3000, Stdout: "hi\n", Stderr: ""},
		},
	}
}

func TestListReturnsFlowsSortedByName(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeFlowFile(t, "zulu", flowBody("zulu"))
	writeFlowFile(t, "alpha", flowBody("alpha"))

	flows, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("got %d flows, want 2", len(flows))
	}
	if flows[0].Name != "alpha" || flows[1].Name != "zulu" {
		t.Fatalf("got %q, %q, want alpha, zulu", flows[0].Name, flows[1].Name)
	}
	if flows[0].Checksum == "" {
		t.Fatalf("flow checksum was not set")
	}
}

func TestListWithoutDirectoryIsEmpty(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	flows, err := List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 0 {
		t.Fatalf("got %d flows, want 0", len(flows))
	}
}

func TestListSurfacesMalformedFlowByFile(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeFlowFile(t, "alpha", flowBody("alpha"))
	writeFlowFile(t, "broken", "name: broken\nsteps: [\n")

	_, err := List()
	if err == nil {
		t.Fatal("expected an error for the malformed flow")
	}
	if !strings.Contains(err.Error(), "broken"+Extension) {
		t.Fatalf("error %q does not name the offending file", err)
	}
}

func TestLoadMissingFlowNamesIt(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	_, err := Load("ghost")
	if err == nil {
		t.Fatal("expected an error for a missing flow")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %q does not name the flow", err)
	}
}

func TestSaveRunThenLoadRunRoundTrips(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	started := time.Date(2026, 8, 18, 21, 4, 11, 221000000, time.UTC)
	want := sampleRun(started)

	path, err := SaveRun(want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want.ID == "" || strings.Contains(want.ID, ":") {
		t.Fatalf("run ID %q is empty or not filename safe", want.ID)
	}
	if filepath.Base(path) != want.ID+".json" {
		t.Fatalf("got path %q, want a file named %q", path, want.ID+".json")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("got mode %v, want -rw-------", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(data), "\n  \"flow\": \"alpha\"") {
		t.Fatalf("artifact is not indented with two spaces:\n%s", data)
	}

	assertRunEqual(t, want, mustLoadRun(t, "alpha", want.ID))
}

func mustLoadRun(t *testing.T, name, runID string) *Run {
	t.Helper()
	got, err := LoadRun(name, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return got
}

func assertRunEqual(t *testing.T, want, got *Run) {
	t.Helper()
	if got.Flow != want.Flow || got.FlowChecksum != want.FlowChecksum {
		t.Fatalf("got flow %q/%q, want %q/%q", got.Flow, got.FlowChecksum, want.Flow, want.FlowChecksum)
	}
	if got.Machine != want.Machine || got.WorkDir != want.WorkDir || got.Status != want.Status {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !got.StartedAt.Equal(want.StartedAt) || !got.FinishedAt.Equal(want.FinishedAt) {
		t.Fatalf("got %v-%v, want %v-%v", got.StartedAt, got.FinishedAt, want.StartedAt, want.FinishedAt)
	}
	if got.ID != want.ID {
		t.Fatalf("got ID %q, want %q", got.ID, want.ID)
	}
	if len(got.Steps) != len(want.Steps) {
		t.Fatalf("got %d steps, want %d", len(got.Steps), len(want.Steps))
	}
	for i := range want.Steps {
		if got.Steps[i] != want.Steps[i] {
			t.Fatalf("step %d: got %+v, want %+v", i, got.Steps[i], want.Steps[i])
		}
	}
}

func seedRuns(t *testing.T, base time.Time, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := SaveRun(sampleRun(base.Add(time.Duration(i) * time.Minute))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestListRunsIsNewestFirstAndHonoursLimit(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	base := time.Date(2026, 8, 18, 21, 0, 0, 0, time.UTC)
	seedRuns(t, base, 3)

	runs, err := ListRuns("alpha", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3", len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if !runs[i-1].StartedAt.After(runs[i].StartedAt) {
			t.Fatalf("runs are not newest first: %v then %v", runs[i-1].StartedAt, runs[i].StartedAt)
		}
	}

	limited, err := ListRuns("alpha", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("got %d runs, want 2", len(limited))
	}
	if !limited[0].StartedAt.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("got %v, want the newest run", limited[0].StartedAt)
	}
}

func TestListRunsWithoutDirectoryIsEmpty(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	runs, err := ListRuns("alpha", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("got %d runs, want 0", len(runs))
	}
}

func TestLoadRunWithoutIDReturnsNewest(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	base := time.Date(2026, 8, 18, 21, 0, 0, 0, time.UTC)
	seedRuns(t, base, 3)

	got := mustLoadRun(t, "alpha", "")
	if !got.StartedAt.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("got %v, want %v", got.StartedAt, base.Add(2*time.Minute))
	}
	if got.ID != newRunID(base.Add(2*time.Minute)) {
		t.Fatalf("got ID %q, want %q", got.ID, newRunID(base.Add(2*time.Minute)))
	}
}

func TestLoadRunMissingIDErrors(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	seedRuns(t, time.Date(2026, 8, 18, 21, 0, 0, 0, time.UTC), 1)

	if _, err := LoadRun("alpha", "nope"); err == nil {
		t.Fatal("expected an error for a missing run")
	}
	if _, err := LoadRun("beta", ""); err == nil {
		t.Fatal("expected an error for a flow with no runs")
	}
}

// TestScaffoldWritesAFlowThatParses keeps the starter file from being one an
// agent must repair before List stops erroring on it.
func TestScaffoldWritesAFlowThatParses(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	path, err := Scaffold("deploy-check")
	if err != nil {
		t.Fatal(err)
	}
	flows, err := List()
	if err != nil {
		t.Fatalf("scaffolded flow does not parse: %v", err)
	}
	if len(flows) != 1 || flows[0].Name != "deploy-check" {
		t.Fatalf("want the scaffolded flow, got %+v", flows)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// TestScaffoldIsNotTrust proves a flow an agent writes still has to be pinned
// by a human before it can run.
func TestScaffoldIsNotTrust(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := Scaffold("deploy-check"); err != nil {
		t.Fatal(err)
	}
	f, err := Load("deploy-check")
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := IsTrusted(f)
	if err != nil {
		t.Fatal(err)
	}
	if trusted {
		t.Fatal("a scaffolded flow must not be trusted")
	}
}

// TestScaffoldRefusesToClobberOrEscape covers the two ways the name argument
// can be hostile: an existing flow, and a path that leaves the flows dir.
func TestScaffoldRefusesToClobberOrEscape(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := Scaffold("deploy-check"); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold("deploy-check"); err == nil {
		t.Fatal("want an error on an existing flow")
	}
	for _, name := range []string{"", ".hidden", "../escape", "nested/name"} {
		if _, err := Scaffold(name); err == nil {
			t.Fatalf("want an error for name %q", name)
		}
	}
}
