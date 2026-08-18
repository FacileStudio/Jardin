package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
)

const wikiPage = `---
title: Sync gotchas
type: tool
---

The sync client reconciles against a local base manifest.

### Conflict backups
A genuine edit-vs-edit conflict keeps a conflict file rather than losing an edit.

### Debounce window
The worker coalesces a burst of writes into one pass per page.
`

// fakeBackend is a Backend that never talks to a model. delay is how long an
// embedding "takes", which is what makes a blocking request path visible.
type fakeBackend struct {
	mu    sync.Mutex
	delay time.Duration
	err   error
	seen  []string
}

func (f *fakeBackend) Embed(ctx context.Context, texts []string) ([]memory.Vector, error) {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	f.mu.Lock()
	f.seen = append(f.seen, texts...)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	vectors := make([]memory.Vector, len(texts))
	for i := range vectors {
		vectors[i] = memory.Vector{1, 0, 0}
	}
	return vectors, nil
}

func (f *fakeBackend) Model(context.Context) (memory.ModelID, error) {
	return memory.ModelID{Name: "fake", Dims: 3}, nil
}

func (f *fakeBackend) embedded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

// fakeStore is a Store that answers from whatever the test put in it, so a
// ranking is a fixture rather than a similarity computation.
type fakeStore struct {
	nearestErr error
	mu         sync.Mutex
	hashes     map[string]string
	nearest    []memory.Scored
	upserted   []memory.Entry
	deleted    []string
}

func (f *fakeStore) Model() memory.ModelID { return memory.ModelID{Name: "fake", Dims: 3} }

func (f *fakeStore) Hashes() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.hashes))
	for k, v := range f.hashes {
		out[k] = v
	}
	return out
}

func (f *fakeStore) Upsert(entries []memory.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted = append(f.upserted, entries...)
	if f.hashes == nil {
		f.hashes = map[string]string{}
	}
	for _, entry := range entries {
		f.hashes[entry.Key] = entry.Hash
	}
	return nil
}

func (f *fakeStore) DeletePaths(paths []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, paths...)
	return nil
}

func (f *fakeStore) Nearest(memory.Vector, int) ([]memory.Scored, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]memory.Scored(nil), f.nearest...), f.nearestErr
}

func (f *fakeStore) upserts() []memory.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]memory.Entry(nil), f.upserted...)
}

// wikiServer builds a server whose common tree holds one wiki page.
func wikiServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "memory", "tools"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	page := filepath.Join(dir, "memory", "tools", "sync.md")
	if err := os.WriteFile(page, []byte(wikiPage), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}
	return New(dir, "")
}

func searchPost(t *testing.T, h http.Handler, body map[string]any) (int, MemorySearchResponse) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/memory/search", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var res MemorySearchResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, res
}

// TestMemorySearchAlwaysAnswers is the feature's invariant: the lexical half
// runs whatever the model is doing, so an absent or broken backend degrades the
// answer and never turns into a 5xx.
func TestMemorySearchAlwaysAnswers(t *testing.T) {
	healthy := &Semantic{Backend: &fakeBackend{}, Store: &fakeStore{}}
	broken := &Semantic{Backend: &fakeBackend{err: errors.New("ollama down")}, Store: &fakeStore{}}

	cases := []struct {
		name     string
		semantic *Semantic
		degraded bool
	}{
		{name: "no backend at all", semantic: nil, degraded: true},
		{name: "backend without a store", semantic: &Semantic{Backend: &fakeBackend{}}, degraded: true},
		{name: "backend that errors", semantic: broken, degraded: true},
		{name: "backend that works", semantic: healthy, degraded: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := wikiServer(t)
			s.Semantic = tc.semantic
			code, res := searchPost(t, s.Handler(), map[string]any{"query": "conflict backup"})

			if code != http.StatusOK {
				t.Fatalf("status %d, want 200", code)
			}
			if res.Degraded != tc.degraded {
				t.Errorf("degraded = %v, want %v", res.Degraded, tc.degraded)
			}
			if len(res.Results) == 0 {
				t.Fatal("lexical half must return results regardless of the model")
			}
			if res.Results[0].Path != filepath.Join("tools", "sync.md") {
				t.Errorf("path = %q", res.Results[0].Path)
			}
			if res.Results[0].Excerpt == "" {
				t.Error("hit carries no excerpt")
			}
		})
	}
}

// TestMemorySearchFusesBothHalves checks the vector half actually reaches the
// ranking: a chunk the lexical half never matches still surfaces.
func TestMemorySearchFusesBothHalves(t *testing.T) {
	s := wikiServer(t)
	chunks := memory.Chunks("tools/sync.md", wikiPage)
	target := chunks[len(chunks)-1]
	store := &fakeStore{nearest: []memory.Scored{
		{Key: scopedKey(".", memory.ChunkKey(target)), Path: target.Path, Line: target.Line, Score: 0.9},
	}}
	s.Semantic = &Semantic{Backend: &fakeBackend{}, Store: store}

	code, res := searchPost(t, s.Handler(), map[string]any{"query": "conflict backup", "limit": 10})
	if code != http.StatusOK || res.Degraded {
		t.Fatalf("status %d degraded %v", code, res.Degraded)
	}
	found := false
	for _, hit := range res.Results {
		if hit.Heading == target.Heading {
			found = true
		}
	}
	if !found {
		t.Fatalf("vector-only chunk %q missing from %+v", target.Heading, res.Results)
	}
}

// TestFuseUsesReciprocalRankFusion pins the ordering for two known rankings.
// "b" leads the second ranking yet loses to "a", which leads the first and is
// second in the other — that is the whole point of fusing on ranks.
func TestFuseUsesReciprocalRankFusion(t *testing.T) {
	idx := chunkIndex{byKey: map[string]memory.Chunk{}}
	for _, key := range []string{"a", "b", "c", "d"} {
		idx.byKey[key] = memory.Chunk{Path: key + ".md", Line: 1, Body: key}
	}
	hits := fuse(idx, [][]string{{"a", "c", "b"}, {"b", "a", "d"}}, 10)

	want := []string{"a.md", "b.md", "c.md", "d.md"}
	if len(hits) != len(want) {
		t.Fatalf("got %d hits, want %d", len(hits), len(want))
	}
	for i, path := range want {
		if hits[i].Path != path {
			t.Fatalf("rank %d = %q, want %q (scores %+v)", i, hits[i].Path, path, hits)
		}
	}
	want0 := 1/float64(rrfK+1) + 1/float64(rrfK+2)
	if got := hits[0].Score; math.Abs(got-want0) > 1e-12 {
		t.Errorf("top score = %v, want %v (1/61 + 1/62)", got, want0)
	}
}

func TestMemorySearchRejectsUnknownSpace(t *testing.T) {
	s := wikiServer(t)
	code, _ := searchPost(t, s.Handler(), map[string]any{"query": "conflict", "space_id": "nope"})
	if code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", code)
	}
}
