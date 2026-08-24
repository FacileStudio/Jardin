package consolidate

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
)

const dedupeFixturePage = `---
title: Mycelium codebase
type: project
---

### The daemon regenerates agent configs every tick unless stamped
**Date**: 2026-08-01
**Source**: direct observation, internal/daemon/daemon.go
The daemon ticks every minute and regenerating agent configs is write-heavy, so
the install step is gated by a stamp file checked every five minutes.
`

func dedupeWiki(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "projects", "mycelium.md")
	if err := os.WriteFile(page, []byte(dedupeFixturePage), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func ollamaServer(t *testing.T, reply string) *Judge {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"response":"` + reply + `"}`)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return NewJudge(srv.URL, "llama3.2:3b")
}

type decideCase struct {
	name       string
	emptyWiki  bool
	newerClaim bool
	judge      *Judge
	candidate  Candidate
	want       Outcome
	wantMatch  bool
}

func decideCaseWiki(t *testing.T, emptyWiki, newerClaim bool) string {
	t.Helper()
	if emptyWiki {
		return t.TempDir()
	}
	dir := dedupeWiki(t)
	if newerClaim {
		page := filepath.Join(dir, "projects", "mycelium.md")
		data := strings.Replace(dedupeFixturePage, "**Date**: 2026-08-01", "**Date**: 2026-08-30", 1)
		if err := os.WriteFile(page, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func checkDecision(t *testing.T, got Decision, tt decideCase) {
	t.Helper()
	if got.Outcome != tt.want {
		t.Fatalf("outcome = %v, want %v (similarity %.2f metric %s)", got.Outcome, tt.want, got.Similarity, got.Metric)
	}
	if (got.Match != nil) != tt.wantMatch {
		t.Fatalf("match present = %v, want %v", got.Match != nil, tt.wantMatch)
	}
	if tt.wantMatch && got.Match.Path != "projects/mycelium.md" {
		t.Fatalf("match path = %q", got.Match.Path)
	}
	if !tt.emptyWiki && got.Metric != metricLexical {
		t.Fatalf("metric = %q, want lexical", got.Metric)
	}
}

var (
	nearDuplicate = Candidate{
		Text:        "The daemon ticks every minute and regenerating agent configs is write-heavy, so the install step is gated by a stamp file checked every five minutes.",
		EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
		Kind:        KindGotcha,
	}
	unrelated = Candidate{
		Text:        "Zebra quantum marmalade escalates seventeen umbrellas beneath the orchard",
		EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
	}
	contradicting = Candidate{
		Text: "The daemon ticks every minute and regenerating agent configs is write-heavy, so the install step is gated by a stamp file checked every five minutes." +
			" That is no longer true: the fix was to drop the stamp gate because regeneration became cheap and idempotent.",
		EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
	}
)

func TestDecideOutcomes(t *testing.T) {
	tests := []decideCase{
		{
			name:      "create: empty wiki has nothing to consolidate against",
			emptyWiki: true,
			candidate: nearDuplicate,
			want:      OutcomeCreate,
			wantMatch: false,
		},
		{
			name:      "create: no strong lexical match",
			judge:     NewJudge("", ""),
			candidate: unrelated,
			want:      OutcomeCreate,
			wantMatch: false,
		},
		{
			name:      "noop: near-duplicate with judge unavailable fails closed on contradiction",
			judge:     NewJudge("", ""),
			candidate: nearDuplicate,
			want:      OutcomeNoop,
			wantMatch: true,
		},
		{
			name:      "noop: near-duplicate the judge finds uncontradicted",
			judge:     ollamaServer(t, "NO"),
			candidate: nearDuplicate,
			want:      OutcomeNoop,
			wantMatch: true,
		},
		{
			name:      "supersede: contradiction confirmed and claim dated older than episode",
			judge:     ollamaServer(t, "YES"),
			candidate: contradicting,
			want:      OutcomeSupersede,
			wantMatch: true,
		},
		{
			name:       "noop: contradiction confirmed but existing claim is newer than the episode",
			newerClaim: true,
			judge:      ollamaServer(t, "YES"),
			candidate:  contradicting,
			want:       OutcomeNoop,
			wantMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := decideCaseWiki(t, tt.emptyWiki, tt.newerClaim)
			d := &Deduper{MemoryPath: dir, Judge: tt.judge}
			got, err := d.Decide(context.Background(), tt.candidate)
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			checkDecision(t, got, tt)
		})
	}
}

type stubBackend struct {
	vectors []memory.Vector
	err     error
}

func (s stubBackend) Embed(ctx context.Context, texts []string) ([]memory.Vector, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.vectors, nil
}

func (stubBackend) Model(ctx context.Context) (memory.ModelID, error) { return memory.ModelID{}, nil }

func embeddingDeduper(t *testing.T, cos float64, judgeReply string) *Deduper {
	t.Helper()
	sin := sqrt1Minus(cos * cos)
	return &Deduper{
		MemoryPath: dedupeWiki(t),
		Backend:    stubBackend{vectors: []memory.Vector{{1, 0}, {1, float32(sin)}}},
		Judge:      ollamaServer(t, judgeReply),
	}
}

func TestDecideEmbeddingThresholds(t *testing.T) {
	tests := []struct {
		name       string
		candidate  float64
		judgeReply string
		want       Outcome
	}{
		{name: "below near-duplicate threshold creates", candidate: 0.80, want: OutcomeCreate},
		{name: "between thresholds noops despite contradiction", candidate: 0.91, judgeReply: "YES", want: OutcomeNoop},
		{name: "at supersede threshold supersedes", candidate: 0.95, judgeReply: "YES", want: OutcomeSupersede},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := embeddingDeduper(t, tt.candidate, tt.judgeReply)
			got, err := d.Decide(context.Background(), Candidate{
				Text:        "anything",
				EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Outcome != tt.want || got.Metric != metricEmbeddings {
				t.Fatalf("outcome = %v metric = %s, want %v embeddings", got.Outcome, got.Metric, tt.want)
			}
		})
	}
}

func TestDecideEmbedFailureFallsBackToLexical(t *testing.T) {
	d := &Deduper{
		MemoryPath: dedupeWiki(t),
		Backend:    stubBackend{err: errors.New("ollama down")},
		Judge:      ollamaServer(t, "NO"),
	}
	got, err := d.Decide(context.Background(), Candidate{
		Text:        "The daemon ticks every minute and regenerating agent configs is write-heavy, so the install step is gated by a stamp file checked every five minutes.",
		EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Metric != metricLexical {
		t.Fatalf("metric = %q, want %q", got.Metric, metricLexical)
	}
	if got.Outcome != OutcomeNoop {
		t.Fatalf("outcome = %v, want noop", got.Outcome)
	}
}

func sqrt1Minus(x float64) float64 { return math.Sqrt(1 - x) }

func TestEarliestTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		refs   []string
		want   string
		wantOK bool
	}{
		{name: "picks oldest across refs", refs: []string{"pi@2026-08-24T10:00:00Z", "pi@2026-08-23T09:00:00Z"}, want: "2026-08-23T09:00:00Z", wantOK: true},
		{name: "gate-style ref without timestamp reports false", refs: []string{"pi/x.jsonl#12"}, wantOK: false},
		{name: "no refs at all", refs: nil, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := earliestTimestamp(Candidate{EpisodeRefs: tt.refs})
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.UTC().Format(time.RFC3339) != tt.want {
				t.Fatalf("ts = %s, want %s", got.UTC().Format(time.RFC3339), tt.want)
			}
		})
	}
}
