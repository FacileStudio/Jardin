package flow

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	for _, step := range f.Steps {
		res := runStep(ctx, step, run.WorkDir, opts.Stream)
		run.Steps = append(run.Steps, res)
		if res.TimedOut {
			run.Status = StatusTimeout
			break
		}
		if res.ExitCode != 0 && !step.AllowFailure {
			run.Status = StatusFailed
			break
		}
	}
	run.FinishedAt = time.Now().UTC()
	return run
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

func runStep(ctx context.Context, step Step, dir string, stream io.Writer) StepResult {
	stepCtx, cancel := context.WithTimeout(ctx, time.Duration(step.EffectiveTimeout())*time.Second)
	defer cancel()

	env := childEnv(step.Env)
	redact := newRedactor(env)

	cmd := exec.CommandContext(stepCtx, "sh", "-c", step.Run)
	cmd.Dir = dir
	cmd.Env = env
	isolate(cmd)
	live := newSink(stream)
	out := newCapture(live, "["+step.Name+"] ", redact)
	errOut := newCapture(live, "["+step.Name+"! ", redact)
	cmd.Stdout = out
	cmd.Stderr = errOut

	started := time.Now()
	err := cmd.Run()
	out.flush()
	errOut.flush()
	stdout, outCut := out.result()
	stderr, errCut := errOut.result()
	return StepResult{
		Name:       step.Name,
		ExitCode:   exitCodeOf(err),
		DurationMS: time.Since(started).Milliseconds(),
		Stdout:     redact(stdout),
		Stderr:     redact(stderr),
		Truncated:  outCut || errCut,
		TimedOut:   errors.Is(stepCtx.Err(), context.DeadlineExceeded),
	}
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

func newRedactor(env []string) func(string) string {
	var values []string
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

type sink struct {
	mu sync.Mutex
	w  io.Writer
}

func newSink(w io.Writer) *sink {
	if w == nil {
		return nil
	}
	return &sink{w: w}
}

func (s *sink) writeString(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(s.w, text)
}

type capture struct {
	mu        sync.Mutex
	buf       []byte
	pending   []byte
	truncated bool
	stream    *sink
	prefix    string
	redact    func(string) string
}

func newCapture(stream *sink, prefix string, redact func(string) string) *capture {
	return &capture{stream: stream, prefix: prefix, redact: redact}
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := MaxStreamBytes - len(c.buf); room > 0 {
		if len(p) > room {
			c.buf = append(c.buf, p[:room]...)
			c.truncated = true
		} else {
			c.buf = append(c.buf, p...)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	c.mirrorLines(p)
	return len(p), nil
}

func (c *capture) mirrorLines(p []byte) {
	if c.stream == nil || len(p) == 0 {
		return
	}
	c.pending = append(c.pending, p...)
	cut := bytes.LastIndexByte(c.pending, '\n')
	if cut < 0 {
		return
	}
	complete := string(c.pending[:cut+1])
	c.pending = append(c.pending[:0], c.pending[cut+1:]...)
	c.emit(complete)
}

func (c *capture) emit(text string) {
	var b strings.Builder
	for _, line := range strings.SplitAfter(c.redact(text), "\n") {
		if line == "" {
			continue
		}
		b.WriteString(c.prefix)
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	c.stream.writeString(b.String())
}

// flush writes whatever the step left without a trailing newline, so a partial
// last line is not swallowed.
func (c *capture) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream == nil || len(c.pending) == 0 {
		return
	}
	c.emit(string(c.pending))
	c.pending = c.pending[:0]
}

func (c *capture) result() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf), c.truncated
}
