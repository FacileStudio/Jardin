package flow

import (
	"strings"
	"testing"
)

func TestExecuteChainsValueForward(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "produce", Run: "echo v0.12.0"},
		{Name: "consume", Needs: map[string]string{"VERSION": "produce.stdout"}, Run: `printf 'shipping %s' "$VERSION"`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q (stderr %q)", run.Status, StatusOK, run.Steps[len(run.Steps)-1].Stderr)
	}
	if got := run.Steps[1].Stdout; got != "shipping v0.12.0" {
		t.Errorf("stdout = %q, want %q — the trailing newline should be trimmed like $(...) does", got, "shipping v0.12.0")
	}
	if got := run.Steps[1].Resolved["VERSION"]; got != "v0.12.0" {
		t.Errorf("artifact recorded VERSION = %q, want %q", got, "v0.12.0")
	}
}

func TestExecuteChainsExitCode(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "probe", Run: "exit 3", AllowFailure: true},
		{Name: "report", Needs: map[string]string{"CODE": "probe.exit_code"}, Run: `printf '%s' "$CODE"`},
	}, Options{})

	if got := run.Steps[1].Stdout; got != "3" {
		t.Errorf("stdout = %q, want %q", got, "3")
	}
}

// A chained value is data, never program text. This is the whole reason values
// travel as environment variables instead of being spliced into the command.
func TestChainedValueIsNotExecuted(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "hostile", Run: `printf '%s' '; rm -rf / ; echo pwned'`},
		{Name: "consume", Needs: map[string]string{"VALUE": "hostile.stdout"}, Run: `printf '%s' "$VALUE"`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q", run.Status, StatusOK)
	}
	if got := run.Steps[1].Stdout; got != "; rm -rf / ; echo pwned" {
		t.Fatalf("stdout = %q, want the value echoed back verbatim", got)
	}
	if strings.Contains(run.Steps[1].Stdout, "pwned\n") {
		t.Fatal("the injected command ran")
	}
}

// Even unquoted, a chained value is not re-scanned for command substitution:
// the shell splits it into words but never runs it. Word-splitting is still the
// flow author's problem, which is why the docs quote every reference.
func TestUnquotedChainedValueIsSplitButNeverExecuted(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "hostile", Run: `printf '%s' '$(echo pwned)'`},
		{Name: "consume", Needs: map[string]string{"VALUE": "hostile.stdout"}, Run: `printf '%s' $VALUE`},
	}, Options{})

	if got := run.Steps[1].Stdout; got != "$(echopwned)" {
		t.Fatalf("stdout = %q, want the substitution left unevaluated", got)
	}
}

func TestTruncatedOutputIsRefusedNotShortened(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "loud", Run: "awk 'BEGIN{while(i++ < 40000) print \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"},
		{Name: "consume", Needs: map[string]string{"VALUE": "loud.stdout"}, Run: "echo unreachable"},
	}, Options{})

	if !run.Steps[0].Truncated {
		t.Fatalf("the producing step was not truncated, so this test proves nothing")
	}
	if run.Status != StatusUnresolved {
		t.Errorf("status = %q, want %q", run.Status, StatusUnresolved)
	}
	if got := run.Steps[1].Stderr; !strings.Contains(got, "truncated") {
		t.Errorf("stderr = %q, want it to say the value was truncated", got)
	}
	if strings.Contains(run.Steps[1].Stdout, "unreachable") {
		t.Error("the consuming step ran on a truncated value")
	}
}

func TestResolvedSecretIsRedactedInTheArtifactButNotInTheStep(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "mint", Run: "echo hunter2hunter2"},
		{Name: "use", Needs: map[string]string{"API_TOKEN": "mint.stdout"}, Run: `test "$API_TOKEN" = hunter2hunter2`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("the step did not receive the real value: status %q", run.Status)
	}
	if got := run.Steps[1].Resolved["API_TOKEN"]; got != "***" {
		t.Errorf("artifact recorded API_TOKEN = %q, want it redacted", got)
	}
}

// A value between MaxValueBytes and MaxStreamBytes is small enough to be
// captured whole and too big to survive execve. Refuse it here, where the
// message can name the step, rather than at exec, where the kernel blames sh.
func TestOversizedValueIsRefusedBeforeExec(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "big", Run: "awk 'BEGIN{while(i++ < 2000) printf \"%s\", \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"},
		{Name: "consume", Needs: map[string]string{"VALUE": "big.stdout"}, Run: "echo unreachable"},
	}, Options{})

	if run.Steps[0].Truncated {
		t.Fatalf("the producing step was truncated, so this exercises the wrong guard")
	}
	if run.Status != StatusUnresolved {
		t.Errorf("status = %q, want %q", run.Status, StatusUnresolved)
	}
	if got := run.Steps[1].Stderr; !strings.Contains(got, "over the") || !strings.Contains(got, "chained value") {
		t.Errorf("stderr = %q, want it to name the limit and suggest a file", got)
	}
	if strings.Contains(run.Steps[1].Stdout, "unreachable") {
		t.Error("the consuming step ran on an oversized value")
	}
}

func TestNulByteIsRefusedBeforeExec(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "binary", Run: `printf 'a\000b'`},
		{Name: "consume", Needs: map[string]string{"VALUE": "binary.stdout"}, Run: "echo unreachable"},
	}, Options{})

	if run.Status != StatusUnresolved {
		t.Fatalf("status = %q, want %q", run.Status, StatusUnresolved)
	}
	if got := run.Steps[1].Stderr; !strings.Contains(got, "NUL byte") {
		t.Errorf("stderr = %q, want it to name the NUL byte", got)
	}
}

func TestValueJustUnderTheLimitStillTravels(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "big", Run: "awk 'BEGIN{while(i++ < 1000) printf \"%s\", \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"},
		{Name: "consume", Needs: map[string]string{"VALUE": "big.stdout"}, Run: `printf '%s' "${#VALUE}"`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q (stderr %q)", run.Status, StatusOK, run.Steps[1].Stderr)
	}
	if got := run.Steps[1].Stdout; got != "40000" {
		t.Errorf("the step saw %q bytes, want 40000", got)
	}
}

func TestExecuteChainsStderr(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "noisy", Run: "echo oops 1>&2"},
		{Name: "consume", Needs: map[string]string{"MSG": "noisy.stderr"}, Run: `printf 'saw %s' "$MSG"`},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q (stderr %q)", run.Status, StatusOK, run.Steps[1].Stderr)
	}
	if got := run.Steps[1].Stdout; got != "saw oops" {
		t.Errorf("stdout = %q, want %q", got, "saw oops")
	}
}

// allow_failure covers a step that ran and returned non-zero. It must not cover
// a step that could never start, which is a defect in the flow itself.
func TestAllowFailureDoesNotRescueAnUnresolvableNeed(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "big", Run: "awk 'BEGIN{while(i++ < 3000) printf \"%s\", \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"},
		{Name: "consume", AllowFailure: true, Needs: map[string]string{"V": "big.stdout"}, Run: "echo hi"},
		{Name: "after", Run: "echo reached"},
	}, Options{})

	if run.Status != StatusUnresolved {
		t.Errorf("status = %q, want %q", run.Status, StatusUnresolved)
	}
	if len(run.Steps) != 3 {
		t.Fatalf("recorded %d steps, want 3 — the producer, the unresolvable step, and the one it blocked", len(run.Steps))
	}
	if run.Steps[2].Stdout != "" || !run.Steps[2].Skipped {
		t.Error("the step after an unresolvable need ran instead of being skipped")
	}
	if !run.Steps[1].NotStarted {
		t.Error("the step that never ran is not marked NotStarted, so exit -1 is its only signal")
	}
}

// A step that ran and failed and a step that never ran must not look the same
// in the artifact — that is the one-bucket-for-two-causes class.
func TestNotStartedSeparatesTheTwoKindsOfMinusOne(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "ran", Run: "exit 1", AllowFailure: true},
		{Name: "binary", Run: `printf 'a\000b'`},
		{Name: "never", Needs: map[string]string{"V": "binary.stdout"}, Run: "echo hi"},
	}, Options{})

	if run.Steps[0].NotStarted {
		t.Error("a step that ran and failed is marked NotStarted")
	}
	if !run.Steps[2].NotStarted {
		t.Error("a step that never ran is not marked NotStarted")
	}
}

func TestTotalChainedBytesAreCapped(t *testing.T) {
	big := "awk 'BEGIN{while(i++ < 1500) printf \"%s\", \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"}'"
	run := execFlow(t, []Step{
		{Name: "a", Run: big},
		{Name: "b", Run: big},
		{Name: "c", Run: big},
		{Name: "d", Run: big},
		{Name: "e", Run: big},
		{Name: "consume", Needs: map[string]string{
			"A": "a.stdout", "B": "b.stdout", "C": "c.stdout", "D": "d.stdout", "E": "e.stdout",
		}, Run: "echo unreachable"},
	}, Options{})

	if run.Status != StatusUnresolved {
		t.Fatalf("status = %q, want %q — five 60KB values exceed the total budget", run.Status, StatusUnresolved)
	}
	if got := run.Steps[5].Stderr; !strings.Contains(got, "total") {
		t.Errorf("stderr = %q, want it to name the total budget", got)
	}
}

// Only the outputs some later step asks for are kept once a step has run.
func TestUnreferencedOutputsAreNotRetained(t *testing.T) {
	f := &Flow{Name: "t", Steps: []Step{
		{Name: "kept", Run: "echo x"},
		{Name: "dropped", Run: "echo y"},
		{Name: "consume", Needs: map[string]string{"V": "kept.stdout"}, Run: "true"},
	}}
	referenced := referencedSteps(f)

	if !referenced["kept"] {
		t.Error("a referenced step is not retained")
	}
	if referenced["dropped"] || referenced["consume"] {
		t.Errorf("unreferenced steps retained: %v", referenced)
	}
}
