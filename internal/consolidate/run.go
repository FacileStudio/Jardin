package consolidate

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
)

const hourlyLimit = time.Hour

// Result reports what one consolidation run did. Dropped counts candidates
// rejected; Reasons aggregates every rejection reason behind them and may
// exceed Dropped. Skipped is non-empty when the stage decided not to run.
type Result struct {
	Created    int      `json:"created"`
	Superseded int      `json:"superseded"`
	Noop       int      `json:"noop"`
	Dropped    int      `json:"dropped"`
	Reasons    []string `json:"reasons,omitempty"`
	Skipped    string   `json:"skipped,omitempty"`
}

// Options tunes a run. Now is a parameter so tests never sleep; Force
// bypasses both the hourly rate limit and the unchanged-events fast path.
type Options struct {
	Force bool
	Now   time.Time
}

// pipeline bundles what one run needs to move candidates through
// judge → gate → dedupe → write.
type pipeline struct {
	ctx     context.Context
	deduper *Deduper
	writer  *Writer
	judge   *Judge
	now     time.Time
	dataDir string
}

func newPipeline(dataDir string, model string) *pipeline {
	memoryPath := filepath.Join(dataDir, "memory")
	judge := NewJudge(ollamaBaseURL(), model)
	return &pipeline{
		ctx:     context.Background(),
		deduper: &Deduper{MemoryPath: memoryPath, Judge: judge},
		writer:  &Writer{MemoryPath: memoryPath},
		judge:   judge,
		dataDir: dataDir,
	}
}

// Run executes the whole pipeline once over the events under dataDir:
// load cursor → read episodes since the watermark → propose → judge → gate →
// dedupe → write. The cursor advances per agent after its write phase; the
// state file only after the whole run, so an interrupted run reprocesses
// what was not persisted.
func Run(dataDir string, opts Options) (Result, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	res := Result{}
	state, err := LoadState(dataDir)
	if err != nil {
		return res, err
	}
	if state == nil {
		state = &State{}
	}
	if skip := rateLimitSkip(state, opts); skip != "" {
		res.Skipped = skip
		return res, nil
	}
	eventsDir := filepath.Join(dataDir, "events")
	cursor, err := LoadCursor(dataDir)
	if err != nil {
		return res, err
	}
	watermark, positioned := earliestWatermark(cursor)
	if positioned && !eventsChangedSince(eventsDir, watermark) &&
		!eventsChangedSince(LocalEventsDir(dataDir), watermark) {
		res.Skipped = "no events file changed since the watermark"
		return res, nil
	}
	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		return res, err
	}
	p := newPipeline(dataDir, cfg.JudgeModel())
	p.now = opts.Now
	if err := p.consolidateAgents(eventsDir, cursor, &res); err != nil {
		return p.fail(res, err)
	}
	if err := cursor.Save(); err != nil {
		return p.fail(res, err)
	}
	if err := saveState(dataDir, State{LastRun: p.now, Result: &res}); err != nil {
		return p.fail(res, err)
	}
	return res, nil
}

// leaksSecret reports whether the gate refused a candidate for carrying a
// credential, which is the one rejection that must short-circuit the judge.
func leaksSecret(rejections []Rejection) bool {
	for _, r := range rejections {
		if r.Rule == RuleSecret {
			return true
		}
	}
	return false
}

// fail records the error and stamps LastRun. A skip must not push the next
// chance to consolidate an hour away, but a failure must: the daemon ticks
// every 60s, and a broken Ollama or an unwritable memory dir would otherwise
// re-run the whole pipeline every minute and append a line to daemon.log each
// time. --force still bypasses the wait.
func (p *pipeline) fail(res Result, err error) (Result, error) {
	if saveErr := saveState(p.dataDir, State{LastRun: p.now, Error: err.Error()}); saveErr != nil {
		return res, fmt.Errorf("%v (also failed to record state: %v)", err, saveErr)
	}
	return res, err
}

func (p *pipeline) consolidateAgents(eventsDir string, cursor *Cursor, res *Result) error {
	agents, err := mergedSourceNames(eventsDir, p.dataDir)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		pos, _ := cursor.PositionFor(agent)
		episodes, err := mergedEpisodes(agent, eventsDir, p.dataDir, pos.Timestamp)
		if err != nil {
			log.Printf("consolidate: skipping agent %s: %v", agent, err)
			continue
		}
		if len(episodes) == 0 {
			continue
		}
		p.consolidateEpisodes(episodes, res)
		last := episodes[len(episodes)-1]
		cursor.MarkProcessed(agent, last.Timestamp, HashLine(strings.Join(last.Refs, "|")))
		if err := cursor.Save(); err != nil {
			return err
		}
	}
	return nil
}

func (p *pipeline) consolidateEpisodes(episodes []Episode, res *Result) {
	for _, cand := range Propose(episodes) {
		p.apply(cand, res)
	}
}

// apply runs the gate before the judge. The judge is a network call, and
// OLLAMA_URL can name a host that is not this machine, so candidate text that
// the secret rule is about to reject must never leave the process first: a rule
// that fires after the exfiltration is not a rule. Every other rejection still
// pays for a verdict, because a run reports all of a candidate's reasons at once.
func (p *pipeline) apply(c Candidate, res *Result) {
	var rejections []string
	gated := Gate(c)
	for _, r := range gated {
		rejections = append(rejections, r.Error())
	}
	if !leaksSecret(gated) {
		if verdict := p.judge.Judge(p.ctx, c); verdict.Verdict != VerdictAccept {
			rejections = append(rejections, fmt.Sprintf("judge refused durability: %q", truncate(c.Text)))
		}
	}
	if len(rejections) > 0 {
		res.Dropped++
		res.Reasons = append(res.Reasons, rejections...)
		return
	}
	if err := p.write(c, res); err != nil {
		res.Dropped++
		res.Reasons = append(res.Reasons, fmt.Sprintf("candidate failed: %v", err))
	}
}

func (p *pipeline) write(c Candidate, res *Result) error {
	dec, err := p.deduper.Decide(p.ctx, c)
	if err != nil {
		return err
	}
	switch dec.Outcome {
	case OutcomeCreate:
		if _, err := p.writer.Create(dec, c, p.now); err != nil {
			return err
		}
		res.Created++
	case OutcomeSupersede:
		if err := p.writer.Supersede(dec, c, p.now); err != nil {
			return err
		}
		res.Superseded++
	default:
		res.Noop++
	}
	return nil
}

// earliestWatermark returns the oldest per-source position, so the cheap stat
// check can only skip when every source is past the newest file change.
func earliestWatermark(c *Cursor) (time.Time, bool) {
	var best time.Time
	found := false
	for _, p := range c.Sources {
		if p.Timestamp.IsZero() {
			continue
		}
		if !found || p.Timestamp.Before(best) {
			best, found = p.Timestamp, true
		}
	}
	return best, found
}

// eventsChangedSince stats every JSONL under the events dir and reports
// whether any of them was modified after the watermark — the cheap check that
// lets an idle machine skip propose/judge/dedupe entirely.
func eventsChangedSince(eventsDir string, watermark time.Time) bool {
	changed := false
	filepath.Walk(eventsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info.ModTime().After(watermark) {
			changed = true
			return filepath.SkipAll
		}
		return nil
	})
	return changed
}

func ollamaBaseURL() string {
	if url := strings.TrimSpace(os.Getenv("OLLAMA_URL")); url != "" {
		return url
	}
	return "http://127.0.0.1:11434"
}
