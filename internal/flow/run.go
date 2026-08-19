package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	tokenEnvVar     = "JARDIN_TOKEN"
	minSecretLength = 4
)

// Options configures one execution. A zero value is valid: the run happens in
// the current directory, with no machine name, and nothing is streamed.
type Options struct {
	WorkDir string
	Machine string
	Stream  io.Writer
	// Parallel caps how many steps run at once. Zero means DefaultParallel.
	Parallel int
}

// Execute runs a flow's steps in order and returns the record of what
// happened. It never returns nil, even when the flow could not start.
func Execute(ctx context.Context, f *Flow, opts Options) *Run {
	run := &Run{
		Flow:         f.Name,
		FlowChecksum: f.Checksum,
		Machine:      opts.Machine,
		WorkDir:      resolveDir(opts.WorkDir),
		StartedAt:    time.Now().UTC(),
		Status:       StatusOK,
		Steps:        make([]StepResult, 0, len(f.Steps)),
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	models, err := preflight(ctx, f)
	if err != nil {
		run.Status = StatusUnresolved
		run.Error = err.Error()
		run.FinishedAt = time.Now().UTC()
		return run
	}

	s := newScheduler(f, run, opts.Parallel, cancel)
	s.models = models
	s.drive(ctx, run.WorkDir, newSink(opts.Stream))
	sortSteps(run.Steps)
	run.FinishedAt = time.Now().UTC()
	return run
}

// unresolved records a step that never started because a value it needed could
// not be produced. allow_failure does not cover it: a reference that cannot be
// satisfied is a defect in the flow, not a step that was allowed to fail.
func unresolved(step Step, err error) StepResult {
	return StepResult{Name: step.Name, ExitCode: -1, Stderr: err.Error(), NotStarted: true}
}

func resolveDir(dir string) string {
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return dir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// stepCommand builds the process a step runs: a shell command, or a model
// extension executed by its runtime.
func stepCommand(ctx context.Context, step Step, model *Model, env map[string]string) (*exec.Cmd, error) {
	if step.Type == "" {
		return exec.CommandContext(ctx, "sh", "-c", step.Run), nil
	}
	if model == nil {
		return nil, fmt.Errorf("step %q has no resolved model for %q", step.Name, step.Type)
	}
	return modelCommand(ctx, model, step, env)
}

// execution is everything a step needs beyond itself: the values resolved from
// earlier steps, where to run, where to mirror output, and the model backing a
// typed step.
type execution struct {
	resolved map[string]string
	// secret holds values that came from ephemeral steps. They are masked
	// everywhere they can surface, not merely left out of one field.
	secret []string
	dir    string
	live   *sink
	model  *Model
}

func runStep(ctx context.Context, step Step, ex execution) (StepResult, output) {
	stepCtx, cancel := context.WithTimeout(ctx, time.Duration(step.EffectiveTimeout())*time.Second)
	defer cancel()

	stepEnv := stepEnvOf(step, ex.resolved)
	env := childEnv(stepEnv)
	redact := newRedactor(env, ex.secret...)

	cmd, cmdErr := stepCommand(stepCtx, step, ex.model, stepEnv)
	if cmdErr != nil {
		return unresolved(step, cmdErr), output{}
	}
	cmd.Dir = ex.dir
	cmd.Env = env
	isolate(cmd)
	mirror := ex.live
	if step.Ephemeral {
		mirror = nil
	}
	out := newCapture(mirror, "["+step.Name+"] ", redact)
	errOut := newCapture(mirror, "["+step.Name+"! ", redact)
	cmd.Stdout = out
	cmd.Stderr = errOut

	started := time.Now()
	err := cmd.Run()
	out.flush()
	errOut.flush()
	stdout, outCut := out.result()
	stderr, errCut := errOut.result()
	code := exitCodeOf(err)
	res := StepResult{
		Name:       step.Name,
		StartedAt:  started.UTC(),
		ExitCode:   code,
		DurationMS: time.Since(started).Milliseconds(),
		Stdout:     redact(stdout),
		Stderr:     redact(stderr),
		Resolved:   redactMap(ex.resolved, redact),
		Truncated:  outCut || errCut,
		TimedOut:   errors.Is(stepCtx.Err(), context.DeadlineExceeded),
	}
	if step.Ephemeral {
		res.Stdout, res.Stderr, res.Ephemeral = "", "", true
	}
	raw := output{Stdout: stdout, Stderr: stderr, ExitCode: code,
		StdoutCut: outCut, StderrCut: errCut, Ephemeral: step.Ephemeral}
	return res, raw
}

func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func childEnv(stepEnv map[string]string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(stepEnv))
	overridden := make(map[string]bool, len(stepEnv))
	for name := range stepEnv {
		overridden[name] = true
	}
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == tokenEnvVar || overridden[name] {
			continue
		}
		out = append(out, entry)
	}
	for name, value := range stepEnv {
		if name == tokenEnvVar {
			continue
		}
		out = append(out, name+"="+value)
	}
	return out
}

func isSecretName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func newRedactor(env []string, extra ...string) func(string) string {
	values := append([]string{}, extra...)
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || len(value) < minSecretLength || !isSecretName(name) {
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return func(s string) string { return s }
	}
	pairs := make([]string, 0, len(values)*2)
	for _, value := range values {
		pairs = append(pairs, value, "***")
	}
	replacer := strings.NewReplacer(pairs...)
	return replacer.Replace
}
