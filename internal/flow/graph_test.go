package flow

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func parseFlow(t *testing.T, body string) (*Flow, error) {
	t.Helper()
	return Parse("t.yml", []byte("name: t\nsteps:\n"+body))
}

// Every flow written before parallelism existed must keep running in file
// order. Absent depends_on is not "no dependencies", it is "the step above".
func TestStepsWithoutDependsOnStayInFileOrder(t *testing.T) {
	f, err := parseFlow(t, "  - name: a\n    run: 'true'\n  - name: b\n    run: 'true'\n  - name: c\n    run: 'true'\n")
	if err != nil {
		t.Fatal(err)
	}
	deps := dependencies(f)
	if got := deps["b"]; len(got) != 1 || got[0] != "a" {
		t.Errorf("b waits on %v, want [a]", got)
	}
	if got := deps["c"]; len(got) != 1 || got[0] != "b" {
		t.Errorf("c waits on %v, want [b]", got)
	}
	if got := deps["a"]; len(got) != 0 {
		t.Errorf("the first step waits on %v, want nothing", got)
	}
}

// An empty depends_on is a declaration, not an omission: it says this step has
// no dependencies and may run at once.
func TestEmptyDependsOnReleasesAStep(t *testing.T) {
	f, err := parseFlow(t, "  - name: a\n    run: 'true'\n  - name: b\n    depends_on: []\n    run: 'true'\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := dependencies(f)["b"]; len(got) != 0 {
		t.Errorf("b waits on %v, want nothing", got)
	}
}

func TestNeedsImpliesADependency(t *testing.T) {
	f, err := parseFlow(t, "  - name: b\n    depends_on: []\n    needs:\n      V: a.stdout\n    run: 'true'\n  - name: a\n    depends_on: []\n    run: 'true'\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := dependencies(f)["b"]; len(got) != 1 || got[0] != "a" {
		t.Errorf("b waits on %v, want [a] — needing an output is a dependency", got)
	}
}

func TestGraphRefusesCyclesAndUnknownSteps(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"cycle": {
			body: "  - name: a\n    depends_on: [b]\n    run: 'true'\n  - name: b\n    depends_on: [a]\n    run: 'true'\n",
			want: "form a cycle",
		},
		"self": {
			body: "  - name: a\n    depends_on: [a]\n    run: 'true'\n",
			want: "depends on itself",
		},
		"unknown": {
			body: "  - name: a\n    depends_on: [ghost]\n    run: 'true'\n",
			want: "not a step in this flow",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseFlow(t, tc.body)
			if err == nil {
				t.Fatal("accepted a flow that cannot run")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// The cycle error names the loop, so the reader knows which steps to edit.
func TestCycleErrorNamesTheLoop(t *testing.T) {
	_, err := parseFlow(t, "  - name: a\n    depends_on: [c]\n    run: 'true'\n  - name: b\n    depends_on: [a]\n    run: 'true'\n  - name: c\n    depends_on: [b]\n    run: 'true'\n")
	if err == nil {
		t.Fatal("accepted a cyclic flow")
	}
	for _, name := range []string{`"a"`, `"b"`, `"c"`} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

// Concurrency is asserted from the recorded times rather than from the clock:
// a step that starts before another has finished overlapped it, and that stays
// true however slow the machine is. A wall-clock bound would only be measuring
// how busy the runner was.
func TestIndependentStepsOverlap(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "a", DependsOn: []string{}, Run: "sleep 1"},
		{Name: "b", DependsOn: []string{}, Run: "sleep 1"},
		{Name: "c", DependsOn: []string{}, Run: "sleep 1"},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q", run.Status, StatusOK)
	}
	if len(run.Steps) != 3 {
		t.Fatalf("recorded %d steps", len(run.Steps))
	}
	first := run.Steps[0]
	firstEnded := first.StartedAt.Add(time.Duration(first.DurationMS) * time.Millisecond)
	for _, later := range run.Steps[1:] {
		if !later.StartedAt.Before(firstEnded) {
			t.Errorf("%q started at %v, after %q finished at %v: they ran in series",
				later.Name, later.StartedAt, first.Name, firstEnded)
		}
	}
}

// The roadmap's acceptance criterion: a failure in one branch does not stop
// independent branches from finishing.
func TestFailureStopsItsBranchAndNothingElse(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "broken", DependsOn: []string{}, Run: "exit 1"},
		{Name: "downstream", DependsOn: []string{"broken"}, Run: "echo should-not-run"},
		{Name: "elsewhere", DependsOn: []string{}, Run: "echo independent"},
	}, Options{})

	byName := map[string]StepResult{}
	for _, s := range run.Steps {
		byName[s.Name] = s
	}
	if run.Status != StatusFailed {
		t.Errorf("status = %q, want %q", run.Status, StatusFailed)
	}
	if !byName["downstream"].Skipped {
		t.Error("the dependent step was not skipped")
	}
	if strings.Contains(byName["downstream"].Stdout, "should-not-run") {
		t.Error("the dependent step ran anyway")
	}
	if got := strings.TrimSpace(byName["elsewhere"].Stdout); got != "independent" {
		t.Errorf("the independent branch did not finish: stdout = %q", got)
	}
	if byName["downstream"].Stderr == "" {
		t.Error("a skipped step does not say which dependency stopped it")
	}
}

// Only the lower bound is asserted: a busy machine can make this slower, but
// nothing can make serialised steps finish faster than their sum.
func TestParallelLimitIsRespected(t *testing.T) {
	start := time.Now()
	run := execFlow(t, []Step{
		{Name: "a", DependsOn: []string{}, Run: "sleep 1"},
		{Name: "b", DependsOn: []string{}, Run: "sleep 1"},
	}, Options{Parallel: 1})
	elapsed := time.Since(start)

	if run.Status != StatusOK {
		t.Fatalf("status = %q", run.Status)
	}
	if elapsed < 2*time.Second {
		t.Errorf("two one-second steps took %v with Parallel=1: the cap was ignored", elapsed)
	}
}

// The artifact is ordered by when each step started, not by when it finished,
// so a parallel run reads as what happened.
func TestArtifactRecordsRealStartOrder(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "slow", DependsOn: []string{}, Run: "sleep 1"},
		{Name: "quick", DependsOn: []string{"slow"}, Run: "true"},
	}, Options{})

	if len(run.Steps) != 2 {
		t.Fatalf("recorded %d steps", len(run.Steps))
	}
	if run.Steps[0].Name != "slow" || run.Steps[1].Name != "quick" {
		t.Errorf("order = %q then %q, want slow then quick", run.Steps[0].Name, run.Steps[1].Name)
	}
	if !run.Steps[0].StartedAt.Before(run.Steps[1].StartedAt) {
		t.Error("start times do not reflect the real order")
	}
}

// Steps finish in whatever order the scheduler gives them, so a run that both
// failed and hit an unresolvable need must not report whichever landed last.
func TestWorstStatusWinsRegardlessOfOrder(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "slowfail", DependsOn: []string{}, Run: "sleep 0.3; exit 1"},
		{Name: "big", DependsOn: []string{}, Run: "awk 'BEGIN{while(i++ < 3000) printf \"%s\", \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"},
		{Name: "consume", DependsOn: []string{"big"}, Needs: map[string]string{"V": "big.stdout"}, Run: "echo hi"},
	}, Options{})

	if run.Status != StatusUnresolved {
		t.Fatalf("status = %q, want %q — the worst outcome, not the last one", run.Status, StatusUnresolved)
	}
	var failed bool
	for _, s := range run.Steps {
		if s.Name == "slowfail" && s.ExitCode == 1 {
			failed = true
		}
	}
	if !failed {
		t.Error("the failing step is missing from the artifact, so the status hides it entirely")
	}
}

// A step marked ephemeral must not reach the terminal either. Keeping a secret
// off disk while printing it to a console that agents and CI capture is not
// keeping it.
func TestEphemeralOutputIsNotStreamedLive(t *testing.T) {
	var live bytes.Buffer
	run := execFlow(t, []Step{
		{Name: "mint", Run: "echo super-secret-token", Ephemeral: true},
		{Name: "loud", Run: "echo ordinary-output"},
	}, Options{Stream: &live})

	if run.Status != StatusOK {
		t.Fatalf("status = %q", run.Status)
	}
	if strings.Contains(live.String(), "super-secret-token") {
		t.Errorf("ephemeral output was streamed to the terminal: %q", live.String())
	}
	if !strings.Contains(live.String(), "ordinary-output") {
		t.Error("suppressing ephemeral output also silenced the ordinary steps")
	}
}
