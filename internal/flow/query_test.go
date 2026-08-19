package flow

import (
	"testing"
	"time"
)

func seedRun(t *testing.T, name, status string, started time.Time) {
	t.Helper()
	r := &Run{
		Flow: name, FlowChecksum: "sha256:x", Machine: "test", WorkDir: "/tmp",
		StartedAt: started, FinishedAt: started.Add(time.Second), Status: status,
		Steps: []StepResult{{Name: "only", ExitCode: 0, StartedAt: started}},
	}
	if status == StatusFailed {
		r.Steps = []StepResult{
			{Name: "broke", ExitCode: 2, StartedAt: started},
			{Name: "downstream", ExitCode: -1, Skipped: true, NotStarted: true, StartedAt: started},
		}
	}
	if _, err := SaveRun(r); err != nil {
		t.Fatal(err)
	}
}

// The roadmap's acceptance criterion: "which runs failed this week, across all
// flows" is one command.
func TestQueryFindsFailuresAcrossEveryFlow(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	now := time.Now().UTC()
	seedRun(t, "alpha", StatusFailed, now.Add(-2*time.Hour))
	seedRun(t, "alpha", StatusOK, now.Add(-3*time.Hour))
	seedRun(t, "beta", StatusFailed, now.Add(-24*time.Hour))
	seedRun(t, "beta", StatusFailed, now.Add(-30*24*time.Hour))

	got, err := Query(QueryOptions{Status: StatusFailed, Since: now.AddDate(0, 0, -7)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d runs, want 2 (the month-old failure is outside the window)", len(got))
	}
	if got[0].Flow != "alpha" || got[1].Flow != "beta" {
		t.Errorf("order = %q, %q; want newest first", got[0].Flow, got[1].Flow)
	}
}

func TestQueryNarrowsToOneFlowAndLimits(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	now := time.Now().UTC()
	seedRun(t, "alpha", StatusOK, now.Add(-time.Hour))
	seedRun(t, "alpha", StatusOK, now.Add(-2*time.Hour))
	seedRun(t, "beta", StatusOK, now.Add(-time.Minute))

	got, err := Query(QueryOptions{Flow: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("found %d runs for alpha, want 2", len(got))
	}
	limited, err := Query(QueryOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].Flow != "beta" {
		t.Fatalf("limit did not keep the newest run: %+v", limited)
	}
}

// A flow can be deleted and its history still answers what happened.
func TestQueryReadsRunsOfFlowsThatNoLongerExist(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	seedRun(t, "deleted", StatusFailed, time.Now().UTC())

	got, err := Query(QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Flow != "deleted" {
		t.Fatalf("history of a removed flow was not readable: %+v", got)
	}
}

func TestQueryOnAnEmptyMachineIsNotAnError(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	got, err := Query(QueryOptions{})
	if err != nil {
		t.Fatalf("querying a machine that has never run a flow errored: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("found %d runs on an empty machine", len(got))
	}
}

// An ephemeral output reaches the step that needs it and never lands on disk.
func TestEphemeralOutputChainsButIsNotRecorded(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "mint", Run: "echo secret-value", Ephemeral: true},
		{Name: "use", Needs: map[string]string{"V": "mint.stdout"}, Run: `test "$V" = secret-value`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("the value did not reach the step that needed it: status %q", run.Status)
	}
	if run.Steps[0].Stdout != "" {
		t.Errorf("ephemeral stdout was recorded: %q", run.Steps[0].Stdout)
	}
	if !run.Steps[0].Ephemeral {
		t.Error("the artifact does not say the output was withheld, so it reads as a step that printed nothing")
	}
}

func TestEphemeralOutputSurvivesARoundTripToDisk(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	run := execFlow(t, []Step{{Name: "mint", Run: "echo secret-value", Ephemeral: true}}, Options{})
	if _, err := SaveRun(run); err != nil {
		t.Fatal(err)
	}
	runs, err := ListRuns("t", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("saved %d runs", len(runs))
	}
	if got := runs[0].Steps[0].Stdout; got != "" {
		t.Errorf("the secret reached disk after all: %q", got)
	}
}
