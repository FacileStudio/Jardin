package flow

import (
	"strings"
	"testing"
)

func parseSteps(t *testing.T, body string) error {
	t.Helper()
	_, err := Parse("t.yml", []byte("name: t\nsteps:\n"+body))
	return err
}

func TestParseRejectsBadNeeds(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"forward reference": {
			body: "  - name: first\n    needs:\n      V: second.stdout\n    run: 'true'\n  - name: second\n    run: 'true'\n",
			want: "does not run before it",
		},
		"unknown step": {
			body: "  - name: first\n    needs:\n      V: nowhere.stdout\n    run: 'true'\n",
			want: "does not run before it",
		},
		"unknown field": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first.duration\n    run: 'true'\n",
			want: "exposes only",
		},
		"not a reference": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first\n    run: 'true'\n",
			want: "is not a <step>.<field> reference",
		},
		"own output": {
			body: "  - name: first\n    needs:\n      V: first.stdout\n    run: 'true'\n",
			want: "needs its own output",
		},
		"collides with env": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    env:\n      V: set\n    needs:\n      V: first.stdout\n    run: 'true'\n",
			want: "in both env and needs",
		},
		"unusable variable name": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      my-var: first.stdout\n    run: 'true'\n",
			want: "not a usable environment variable name",
		},
		"binds the token": {
			body: "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      JARDIN_TOKEN: first.stdout\n    run: 'true'\n",
			want: "may not bind JARDIN_TOKEN",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := parseSteps(t, tc.body)
			if err == nil {
				t.Fatalf("accepted a flow that should not parse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseAcceptsBackwardReference(t *testing.T) {
	body := "  - name: first\n    run: 'true'\n  - name: second\n    needs:\n      V: first.stdout\n      CODE: first.exit_code\n    run: 'true'\n"
	if err := parseSteps(t, body); err != nil {
		t.Fatalf("rejected a valid flow: %v", err)
	}
}

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
	if run.Status != StatusFailed {
		t.Errorf("status = %q, want %q", run.Status, StatusFailed)
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
	if run.Status != StatusFailed {
		t.Errorf("status = %q, want %q", run.Status, StatusFailed)
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

	if run.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", run.Status, StatusFailed)
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
