package flow

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// DefaultParallel bounds how many steps run at once when a flow does not say.
// Steps are shell commands, so an unbounded fan-out is a fork bomb wearing a
// YAML hat.
const DefaultParallel = 4

// outcome is one finished step handed back to the scheduler.
type outcome struct {
	name   string
	result StepResult
	raw    output
}

// scheduler walks the dependency graph, running every step whose dependencies
// have finished. It is the whole of the parallel execution model: steps with no
// edge between them run together, and a step whose dependency failed never
// starts rather than failing for a reason that is not its own.
type scheduler struct {
	flow     *Flow
	deps     map[string][]string
	retain   map[string]bool
	limit    int
	run      *Run
	outputs  map[string]output
	done     map[string]bool
	blocked  map[string]bool
	started  map[string]bool
	inflight int
	results  chan outcome
	cancel   context.CancelFunc
	stopped  bool
	models   map[string]*Model
}

func newScheduler(f *Flow, run *Run, limit int, cancel context.CancelFunc) *scheduler {
	if limit <= 0 {
		limit = DefaultParallel
	}
	return &scheduler{
		flow: f, deps: dependencies(f), retain: referencedSteps(f), limit: limit,
		run: run, outputs: make(map[string]output), done: make(map[string]bool),
		blocked: make(map[string]bool), started: make(map[string]bool),
		results: make(chan outcome), cancel: cancel,
	}
}

// drive runs the flow to completion and returns when no step can make progress.
//
// Only wait on the results channel when something is actually running: a sweep
// that only skipped steps changes the graph without producing a result, and
// waiting for one then would block for good.
func (s *scheduler) drive(ctx context.Context, dir string, stream *sink) {
	for len(s.done) < len(s.flow.Steps) {
		progressed := s.sweep(ctx, dir, stream)
		if s.inflight > 0 {
			s.collect()
			continue
		}
		if !progressed {
			return
		}
	}
}

// sweep starts or skips every step that has become ready, and reports whether
// it changed anything — a pass that neither launches nor skips means the rest
// of the graph is waiting on steps already in flight.
func (s *scheduler) sweep(ctx context.Context, dir string, stream *sink) bool {
	changed := false
	for _, step := range s.flow.Steps {
		if s.started[step.Name] || s.done[step.Name] || !s.ready(step) {
			continue
		}
		if reason := s.blockedBy(step); reason != "" {
			s.skip(step, reason)
			changed = true
			continue
		}
		if s.stopped {
			s.skip(step, "the run stopped before this step could start")
			changed = true
			continue
		}
		if s.inflight >= s.limit {
			break
		}
		s.launch(ctx, step, dir, stream)
		changed = true
	}
	return changed
}

func (s *scheduler) ready(step Step) bool {
	for _, name := range s.deps[step.Name] {
		if !s.done[name] {
			return false
		}
	}
	return true
}

// blockedBy names the dependency that stops this step from running, if any.
func (s *scheduler) blockedBy(step Step) string {
	for _, name := range s.deps[step.Name] {
		if s.blocked[name] {
			return fmt.Sprintf("it depends on %q, which did not succeed", name)
		}
	}
	return ""
}

func (s *scheduler) launch(ctx context.Context, step Step, dir string, stream *sink) {
	resolved, err := resolve(step, s.outputs)
	if err != nil {
		s.record(outcome{name: step.Name, result: unresolved(step, err)})
		return
	}
	s.started[step.Name] = true
	s.inflight++
	go func() {
		res, raw := runStep(ctx, step, resolved, dir, stream, s.models[step.Type])
		s.results <- outcome{name: step.Name, result: res, raw: raw}
	}()
}

func (s *scheduler) skip(step Step, reason string) {
	s.record(outcome{name: step.Name, result: skipped(step, reason)})
}

func (s *scheduler) collect() {
	out := <-s.results
	s.inflight--
	s.record(out)
}

// record files a finished step and updates what the rest of the graph may do.
//
// A step that never ran still gets a start time, stamped at the moment the
// decision was made. Without it the artifact sorts every skipped step to the
// front, since the zero time precedes everything that actually happened.
func (s *scheduler) record(out outcome) {
	if out.result.StartedAt.IsZero() {
		out.result.StartedAt = time.Now().UTC()
	}
	s.started[out.name] = true
	s.done[out.name] = true
	if s.retain[out.name] {
		s.outputs[out.name] = out.raw
	}
	s.run.Steps = append(s.run.Steps, out.result)
	s.judge(out)
}

// judge decides what one step's ending means for the run. A timeout stops
// everything, as it always has. Anything else marks the step blocked so its
// dependents are skipped, while branches that never depended on it finish.
func (s *scheduler) judge(out outcome) {
	res := out.result
	step := s.step(out.name)
	switch {
	case res.TimedOut:
		s.worsen(StatusTimeout)
		s.stopped = true
		s.cancel()
	case res.NotStarted && !res.Skipped:
		s.blocked[out.name] = true
		s.worsen(StatusUnresolved)
	case res.Skipped:
		s.blocked[out.name] = true
	case res.ExitCode != 0 && !step.AllowFailure:
		s.blocked[out.name] = true
		s.worsen(StatusFailed)
	}
}

// statusRank orders how badly a run ended. Steps finish in whatever order the
// scheduler gives them, so a status that is simply assigned would report
// whichever step happened to land last — the same flow could describe itself
// differently on two runs.
func statusRank(status string) int {
	switch status {
	case StatusFailed:
		return 1
	case StatusUnresolved:
		return 2
	case StatusTimeout:
		return 3
	default:
		return 0
	}
}

// worsen moves the run's status towards the worst thing that happened to it,
// and never back, so the result does not depend on scheduling.
func (s *scheduler) worsen(status string) {
	if statusRank(status) > statusRank(s.run.Status) {
		s.run.Status = status
	}
}

func (s *scheduler) step(name string) Step {
	for _, step := range s.flow.Steps {
		if step.Name == name {
			return step
		}
	}
	return Step{}
}

// sortSteps puts the artifact back in the order the steps actually started, so
// a parallel run reads as what happened rather than as what finished first.
func sortSteps(steps []StepResult) {
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].StartedAt.Before(steps[j].StartedAt)
	})
}

// skipped records a step that never ran because something it depends on did
// not succeed. It is not a failure of this step and does not get its exit code.
func skipped(step Step, reason string) StepResult {
	return StepResult{Name: step.Name, ExitCode: -1, Stderr: reason, NotStarted: true, Skipped: true}
}
