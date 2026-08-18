package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

const (
	rrfK              = 60
	defaultHitLimit   = 10
	maxHitLimit       = 50
	excerptLimit      = 240
	embedQueryTimeout = 5 * time.Second
)

// Semantic is the vector half of memory search: the embedding backend, the
// index it fills, and the worker that keeps that index current. A nil
// *Semantic is the dormant configuration, and every path through search treats
// it as normal rather than as a failure.
type Semantic struct {
	Backend memory.Backend
	Store   memory.Store
	worker  *EmbedWorker
	counts  chunkCounts
}

// MemorySearchRequest is the POST /api/memory/search body. An empty SpaceID
// searches the common tree; anything else is checked against membership before
// it is used.
type MemorySearchRequest struct {
	Query   string `json:"query"`
	SpaceID string `json:"space_id"`
	Limit   int    `json:"limit"`
}

// MemoryHit is one ranked chunk of the wiki. Score is the fused rank score and
// is comparable only against the other hits of the same query.
type MemoryHit struct {
	Path    string  `json:"path"`
	Heading string  `json:"heading"`
	Line    int     `json:"line"`
	Score   float64 `json:"score"`
	Excerpt string  `json:"excerpt"`
}

// MemorySearchResponse carries the ranked hits. Degraded reports that the
// vector half did not contribute, so a caller knows the answer is lexical-only
// rather than simply thin.
type MemorySearchResponse struct {
	Results  []MemoryHit `json:"results"`
	Degraded bool        `json:"degraded"`
}

// memorySearchPost answers the hybrid search. The lexical half always runs; the
// vector half is best-effort, so an absent, unreachable or broken model
// degrades the answer instead of failing the request. Search never depends on a
// model being up.
func (s *Server) memorySearchPost(w http.ResponseWriter, r *http.Request) {
	var req MemorySearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("invalid body"))
		return
	}
	root, ok := s.memoryScope(w, r, req.SpaceID)
	if !ok {
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		httpjson.WriteJSON(w, http.StatusOK, MemorySearchResponse{Results: []MemoryHit{}})
		return
	}

	dir := filepath.Join(root, "memory")
	index := chunkIndexOf(dir)
	lexical := s.lexicalRanking(dir, query, index)
	vector, degraded := s.vectorRanking(r.Context(), query, s.treeScope(root), index)

	httpjson.WriteJSON(w, http.StatusOK, MemorySearchResponse{
		Results:  fuse(index, [][]string{lexical, vector}, hitLimit(req.Limit)),
		Degraded: degraded,
	})
}

// memoryScope resolves the tree to search. The space id arrives in the body
// rather than the query string, so it is put back where scopeRoot looks for it
// instead of restating the membership guard: one guard, one place.
func (s *Server) memoryScope(w http.ResponseWriter, r *http.Request, spaceID string) (string, bool) {
	if spaceID == "" {
		return s.scopeRoot(w, r)
	}
	scoped := r.Clone(r.Context())
	query := scoped.URL.Query()
	query.Set("space_id", spaceID)
	scoped.URL.RawQuery = query.Encode()
	return s.scopeRoot(w, scoped)
}

// chunkIndex is the chunk table both halves rank over. The lexical half scores
// lines, the vector half scores chunks, so lines are folded onto the chunk that
// contains them before the two rankings can be fused at all.
type chunkIndex struct {
	byKey  map[string]memory.Chunk
	byPath map[string][]memory.Chunk
}

func chunkIndexOf(dir string) chunkIndex {
	index := chunkIndex{byKey: map[string]memory.Chunk{}, byPath: map[string][]memory.Chunk{}}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		for _, c := range memory.Chunks(rel, string(data)) {
			index.byKey[memory.ChunkKey(c)] = c
			index.byPath[rel] = append(index.byPath[rel], c)
		}
		return nil
	})
	return index
}

// keyAt names the chunk a matching line belongs to: the last chunk starting at
// or before it, since Chunks emits them in file order.
func (idx chunkIndex) keyAt(path string, line int) (string, bool) {
	list := idx.byPath[path]
	if len(list) == 0 {
		return "", false
	}
	best := list[0]
	for _, c := range list {
		if c.Line <= line {
			best = c
		}
	}
	return memory.ChunkKey(best), true
}

// lexicalRanking is the half that must always run. A failure here is logged and
// yields an empty ranking rather than an error response, because a wiki that
// cannot be walked is still not a reason to 500 a search.
func (s *Server) lexicalRanking(dir, query string, idx chunkIndex) []string {
	results, err := memory.SearchChunks(dir, query)
	if err != nil {
		s.Log.Error("memory search: lexical half failed", slog.Any("error", err))
		return nil
	}
	seen := make(map[string]bool, len(results))
	var ranked []string
	for _, res := range results {
		key, ok := idx.keyAt(res.Path, res.Line)
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		ranked = append(ranked, key)
	}
	return ranked
}

// vectorRanking is the best-effort half. It reports degraded whenever it could
// not contribute — no backend, no index, an unreachable model, an empty answer
// — and hits are kept only when they belong to the tree being searched, so one
// shared index can never leak a space's chunks into a common-tree search.
func (s *Server) vectorRanking(ctx context.Context, query, scope string, idx chunkIndex) ([]string, bool) {
	if s.Semantic == nil || s.Semantic.Backend == nil || s.Semantic.Store == nil {
		return nil, true
	}
	embedCtx, cancel := context.WithTimeout(ctx, embedQueryTimeout)
	defer cancel()
	vectors, err := s.Semantic.Backend.Embed(embedCtx, []string{query})
	if err != nil || len(vectors) == 0 {
		s.Log.Warn("memory search: vector half degraded", slog.Any("error", err))
		return nil, true
	}
	hits, err := s.Semantic.Store.Nearest(vectors[0], maxHitLimit)
	if err != nil {
		s.Log.Warn("memory search: vector store degraded", slog.Any("error", err))
		return nil, true
	}
	var ranked []string
	for _, hit := range hits {
		key, ok := unscope(scope, hit.Key)
		if !ok {
			continue
		}
		if _, known := idx.byKey[key]; known {
			ranked = append(ranked, key)
		}
	}
	return ranked, false
}

// fuse combines rankings with Reciprocal Rank Fusion: every ranking contributes
// 1/(k+rank) for each key it lists. Ranks, not raw scores — BM25 weight and
// cosine similarity are not on comparable scales, so adding them would let one
// half's units decide the order.
func fuse(idx chunkIndex, rankings [][]string, limit int) []MemoryHit {
	scores := map[string]float64{}
	for _, ranking := range rankings {
		for rank, key := range ranking {
			scores[key] += 1 / float64(rrfK+rank+1)
		}
	}
	hits := make([]MemoryHit, 0, len(scores))
	for key, score := range scores {
		c := idx.byKey[key]
		hits = append(hits, MemoryHit{
			Path:    c.Path,
			Heading: c.Heading,
			Line:    c.Line,
			Score:   score,
			Excerpt: excerptOf(c.Body),
		})
	}
	sort.Slice(hits, func(i, j int) bool { return betterHit(hits[i], hits[j]) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// betterHit is a total order, so a ranking built from a map still comes out the
// same on every run.
func betterHit(a, b MemoryHit) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Line < b.Line
}

func excerptOf(body string) string {
	flat := strings.Join(strings.Fields(body), " ")
	runes := []rune(flat)
	if len(runes) <= excerptLimit {
		return flat
	}
	return strings.TrimSpace(string(runes[:excerptLimit])) + "…"
}

func hitLimit(limit int) int {
	if limit <= 0 {
		return defaultHitLimit
	}
	if limit > maxHitLimit {
		return maxHitLimit
	}
	return limit
}
