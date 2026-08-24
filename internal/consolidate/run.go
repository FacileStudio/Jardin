package consolidate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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

// State is the persisted outcome of the last run, read by doctor and used
// for the hourly rate limit. LastRun is only stamped by full runs: a skip
// must never push the next chance to consolidate an hour away.
type State struct {
	LastRun time.Time `json:"last_run"`
	Error   string    `json:"error,omitempty"`
	Result  *Result   `json:"result,omitempty"`
}

// StatePath is where the last-run state lives under the data dir.
func StatePath(dataDir string) string {
	return filepath.Join(dataDir, ".consolidate-run.json")
}

// LoadState reads the last-run state, returning nil when none exists yet.
func LoadState(dataDir string) (*State, error) {
	data, err := os.ReadFile(StatePath(dataDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
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

func rateLimitSkip(state *State, opts Options) string {
	now := opts.Now
	if !opts.Force && now.Sub(state.LastRun) < hourlyLimit {
		return fmt.Sprintf("last run %s ago is inside the %s rate limit",
			now.Sub(state.LastRun).Truncate(time.Second), hourlyLimit)
	}
	return ""
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
func (p *pipeline) fail(res Result, err error) (Result, error) {
	if saveErr := saveState(p.dataDir, State{Error: err.Error()}); saveErr != nil {
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

func (p *pipeline) apply(c Candidate, res *Result) {
	var rejections []string
	if verdict := p.judge.Judge(p.ctx, c); verdict.Verdict != VerdictAccept {
		rejections = append(rejections, fmt.Sprintf("judge refused durability: %q", truncate(c.Text)))
	}
	for _, r := range Gate(c) {
		rejections = append(rejections, r.Error())
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

// eventSources lists the agent directories under the events dir, sorted for
// deterministic runs; a missing events dir means nothing to do.
func eventSources(eventsDir string) ([]string, error) {
	entries, err := os.ReadDir(eventsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, e := range entries {
		if e.IsDir() {
			agents = append(agents, e.Name())
		}
	}
	sort.Strings(agents)
	return agents, nil
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

func saveState(dataDir string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := StatePath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// mergedSourceNames lists agents present in either the synced events dir or
// the local-only dir, sorted and deduplicated.
func mergedSourceNames(eventsDir, dataDir string) ([]string, error) {
	synced, err := eventSources(eventsDir)
	if err != nil {
		return nil, err
	}
	local, err := eventSources(LocalEventsDir(dataDir))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(synced)+len(local))
	for _, a := range append(synced, local...) {
		seen[a] = true
	}
	names := make([]string, 0, len(seen))
	for a := range seen {
		names = append(names, a)
	}
	sort.Strings(names)
	return names, nil
}

// mergedEpisodes reads one agent's episodes from both the synced events dir
// and the local-only dir (where harnesses log message text that must never
// sync), sorted by timestamp.
func mergedEpisodes(agent, eventsDir, dataDir string, watermark time.Time) ([]Episode, error) {
	synced, err := (&EventsSource{Agent: agent, EventsDir: eventsDir}).Since(watermark)
	if err != nil {
		return nil, err
	}
	local, err := (&EventsSource{Agent: agent, EventsDir: LocalEventsDir(dataDir)}).Since(watermark)
	if err != nil {
		return nil, err
	}
	episodes := append(synced, local...)
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].Timestamp.Before(episodes[j].Timestamp) })
	return episodes, nil
}
