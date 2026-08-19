package flow

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func execFlow(t *testing.T, steps []Step, opts Options) *Run {
	t.Helper()
	f := &Flow{Name: "t", Checksum: "sha256:test", Steps: steps}
	run := Execute(context.Background(), f, opts)
	if run == nil {
		t.Fatal("Execute returned nil")
	}
	return run
}

func TestExecuteOrderAndExitCodes(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "one", Run: "echo hi"},
		{Name: "two", Run: "echo there 1>&2"},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q", run.Status, StatusOK)
	}
	if len(run.Steps) != 2 {
		t.Fatalf("ran %d steps, want 2", len(run.Steps))
	}
	if run.Steps[0].Name != "one" || run.Steps[1].Name != "two" {
		t.Fatalf("steps out of order: %q then %q", run.Steps[0].Name, run.Steps[1].Name)
	}
	if got := strings.TrimSpace(run.Steps[0].Stdout); got != "hi" {
		t.Errorf("stdout = %q, want %q", got, "hi")
	}
	if got := strings.TrimSpace(run.Steps[1].Stderr); got != "there" {
		t.Errorf("stderr = %q, want %q", got, "there")
	}
	if run.Flow != "t" || run.FlowChecksum != "sha256:test" || run.WorkDir == "" {
		t.Errorf("run metadata not recorded: %+v", run)
	}
	if run.FinishedAt.Before(run.StartedAt) {
		t.Error("FinishedAt precedes StartedAt")
	}
}

func TestExecuteStopsOnFailure(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "boom", Run: "exit 3"},
		{Name: "never", Run: "echo nope"},
	}, Options{})

	if run.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", run.Status, StatusFailed)
	}
	if len(run.Steps) != 2 {
		t.Fatalf("recorded %d steps, want 2 — the failure and the step it blocked", len(run.Steps))
	}
	if run.Steps[0].ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", run.Steps[0].ExitCode)
	}
	if !run.Steps[1].Skipped {
		t.Error("the blocked step is not marked skipped, so the artifact hides why it did not run")
	}
	if run.Steps[1].Stdout != "" {
		t.Error("the blocked step produced output, so it actually ran")
	}
}

func TestExecuteAllowFailure(t *testing.T) {
	run := execFlow(t, []Step{
		{Name: "boom", Run: "exit 3", AllowFailure: true},
		{Name: "after", Run: "echo still here"},
	}, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, want %q", run.Status, StatusOK)
	}
	if len(run.Steps) != 2 {
		t.Fatalf("ran %d steps, want 2", len(run.Steps))
	}
	if run.Steps[1].ExitCode != 0 {
		t.Errorf("second step exit code = %d, want 0", run.Steps[1].ExitCode)
	}
}

func TestExecuteTimeout(t *testing.T) {
	started := time.Now()
	run := execFlow(t, []Step{
		{Name: "slow", Run: "sleep 5", Timeout: 1},
		{Name: "never", Run: "echo nope"},
	}, Options{})

	if run.Status != StatusTimeout {
		t.Fatalf("status = %q, want %q", run.Status, StatusTimeout)
	}
	if !run.Steps[0].TimedOut {
		t.Error("step not marked as timed out")
	}
	if len(run.Steps) != 2 {
		t.Fatalf("recorded %d steps, want 2 — the timeout and the step it blocked", len(run.Steps))
	}
	if !run.Steps[1].Skipped {
		t.Error("the step after a timeout is not marked skipped")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("timeout took %s, want about 1s", elapsed)
	}
}

func TestExecuteRedactsSecrets(t *testing.T) {
	var stream bytes.Buffer
	run := execFlow(t, []Step{{
		Name: "leak",
		Run:  "echo $API_TOKEN; echo $PLAIN_NAME",
		Env:  map[string]string{"API_TOKEN": "supersecretvalue", "PLAIN_NAME": "harmlessvalue"},
	}}, Options{Stream: &stream})

	out := run.Steps[0].Stdout
	if strings.Contains(out, "supersecretvalue") {
		t.Errorf("secret survived redaction: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("stdout = %q, want a masked value", out)
	}
	if !strings.Contains(out, "harmlessvalue") {
		t.Errorf("non-secret var was masked: %q", out)
	}
	if strings.Contains(stream.String(), "supersecretvalue") {
		t.Errorf("secret survived in the streamed copy: %q", stream.String())
	}
	if !strings.Contains(stream.String(), "[leak]") {
		t.Errorf("streamed copy is not prefixed: %q", stream.String())
	}
}

func TestExecuteStripsJardinToken(t *testing.T) {
	t.Setenv("JARDIN_TOKEN", "leakme")
	run := execFlow(t, []Step{{Name: "env", Run: `echo "[$JARDIN_TOKEN]"`}}, Options{})

	if strings.Contains(run.Steps[0].Stdout, "leakme") {
		t.Fatalf("JARDIN_TOKEN reached the child: %q", run.Steps[0].Stdout)
	}
	if got := strings.TrimSpace(run.Steps[0].Stdout); got != "[]" {
		t.Errorf("stdout = %q, want %q", got, "[]")
	}
}

func TestExecuteTruncatesOversizedOutput(t *testing.T) {
	run := execFlow(t, []Step{{
		Name: "flood",
		Run:  "head -c 1200000 /dev/zero | tr '\\0' 'a'",
	}}, Options{})

	res := run.Steps[0]
	if !res.Truncated {
		t.Fatal("oversized output was not flagged as truncated")
	}
	if len(res.Stdout) != MaxStreamBytes {
		t.Errorf("stored %d bytes, want %d", len(res.Stdout), MaxStreamBytes)
	}
}

func TestExecuteUsesWorkDir(t *testing.T) {
	dir := t.TempDir()
	run := execFlow(t, []Step{{Name: "pwd", Run: "pwd"}}, Options{WorkDir: dir})

	if run.WorkDir != dir {
		t.Errorf("WorkDir = %q, want %q", run.WorkDir, dir)
	}
	if got := strings.TrimSpace(run.Steps[0].Stdout); got != dir {
		t.Errorf("step ran in %q, want %q", got, dir)
	}
}

// TestCaptureRedactsAcrossChunkBoundaries proves a secret split over two writes
// is still masked on the live stream. os/exec copies in fixed-size reads, so a
// long-running step will split a value sooner or later.
func TestCaptureRedactsAcrossChunkBoundaries(t *testing.T) {
	var live strings.Builder
	redact := newRedactor([]string{"API_TOKEN=supersecretvalue"})
	c := newCapture(newSink(&live), "[s] ", redact)

	if _, err := c.Write([]byte("prefix super")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("secretvalue suffix\n")); err != nil {
		t.Fatal(err)
	}
	c.flush()

	if got := live.String(); strings.Contains(got, "supersecretvalue") {
		t.Fatalf("secret leaked to the live stream: %q", got)
	}
	if got := live.String(); !strings.Contains(got, "***") {
		t.Fatalf("want the masked value, got %q", got)
	}
}

// TestCaptureFlushesAPartialLine keeps a step's last line from vanishing when
// it never printed a newline.
func TestCaptureFlushesAPartialLine(t *testing.T) {
	var live strings.Builder
	c := newCapture(newSink(&live), "[s] ", func(s string) string { return s })
	if _, err := c.Write([]byte("no trailing newline")); err != nil {
		t.Fatal(err)
	}
	if live.String() != "" {
		t.Fatalf("a partial line must wait for flush, got %q", live.String())
	}
	c.flush()
	if got := live.String(); got != "[s] no trailing newline\n" {
		t.Fatalf("got %q", got)
	}
}
