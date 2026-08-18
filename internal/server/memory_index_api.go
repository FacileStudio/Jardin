package server

import (
	"math"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/FacileStudio/Jardin/internal/memory"
	"github.com/FacileStudio/tronc/httpjson"
)

// chunkCountTTL is how long a tree's chunk count is trusted. The dashboard
// polls every two seconds while indexing, and counting chunks means walking
// every page: without this the poll would re-read the whole wiki thirty times a
// minute to learn a number that moves once a sync.
const chunkCountTTL = 10 * time.Second

// MemoryIndexStatusResponse is what the dashboard renders a progress bar from.
// Enabled is false with everything else zeroed when no embedding backend is
// configured — a dormant index is a state to display, not an error to raise.
type MemoryIndexStatusResponse struct {
	Enabled         bool           `json:"enabled"`
	Model           memory.ModelID `json:"model"`
	Store           string         `json:"store"`
	TotalChunks     int            `json:"total_chunks"`
	IndexedChunks   int            `json:"indexed_chunks"`
	PendingPaths    int            `json:"pending_paths"`
	Indexing        bool           `json:"indexing"`
	StartedAt       string         `json:"started_at"`
	UpdatedAt       string         `json:"updated_at"`
	ChunksPerSecond float64        `json:"chunks_per_second"`
	ETASeconds      int            `json:"eta_seconds"`
	LastError       string         `json:"last_error"`
}

// memoryIndexStatus reports how far the vector index has got. The space guard
// runs before the dormant check, so an unknown space is refused whether or not
// this deployment embeds anything.
func (s *Server) memoryIndexStatus(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	if s.Semantic == nil {
		httpjson.WriteJSON(w, http.StatusOK, MemoryIndexStatusResponse{})
		return
	}
	dir := filepath.Join(root, "memory")
	httpjson.WriteJSON(w, http.StatusOK, s.Semantic.indexStatus(dir, time.Now().UTC()))
}

// indexStatus assembles the snapshot. Every number is read once, from the same
// instant, so the rate and the ETA describe one moment rather than three.
func (sem *Semantic) indexStatus(dir string, now time.Time) MemoryIndexStatusResponse {
	status := sem.worker.Status()
	res := MemoryIndexStatusResponse{
		Enabled:       true,
		Store:         storeKind(sem.Store),
		TotalChunks:   sem.counts.total(dir, now),
		IndexedChunks: indexedChunks(sem.Store),
		PendingPaths:  status.Pending,
		Indexing:      status.Indexing,
		StartedAt:     stamp(status.StartedAt),
		UpdatedAt:     stamp(status.UpdatedAt),
		LastError:     status.LastError,
	}
	if sem.Store != nil {
		res.Model = sem.Store.Model()
	}
	res.ChunksPerSecond, res.ETASeconds = indexPace(status, res.TotalChunks-res.IndexedChunks, now)
	return res
}

// indexPace measures the current pass rather than the store's lifetime: a full
// index that has been running four minutes is the only sample that predicts the
// remaining thirty-six. An idle worker has no pace, and reports none.
func indexPace(status EmbedStatus, remaining int, now time.Time) (float64, int) {
	if !status.Indexing || status.Processed <= 0 || status.StartedAt.IsZero() {
		return 0, 0
	}
	elapsed := now.Sub(status.StartedAt).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	rate := float64(status.Processed) / elapsed
	rounded := math.Round(rate*100) / 100
	if remaining <= 0 {
		return rounded, 0
	}
	return rounded, int(math.Round(float64(remaining) / rate))
}

// storeKind names the vector store the index lives in. An unrecognised
// implementation is reported as empty rather than guessed at.
func storeKind(store memory.Store) string {
	switch store.(type) {
	case *memory.FlatStore:
		return "flat"
	case *memory.QdrantStore:
		return "qdrant"
	default:
		return ""
	}
}

func indexedChunks(store memory.Store) int {
	if store == nil {
		return 0
	}
	return len(store.Hashes())
}

func stamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// chunkCounts caches each tree's chunk count for chunkCountTTL. The walk runs
// under the lock, so a burst of pollers costs one walk between them instead of
// one each.
type chunkCounts struct {
	mu     sync.Mutex
	byRoot map[string]chunkCount
}

type chunkCount struct {
	total int
	at    time.Time
}

func (c *chunkCounts) total(dir string, now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hit, ok := c.byRoot[dir]; ok && now.Sub(hit.at) < chunkCountTTL {
		return hit.total
	}
	total := len(chunkIndexOf(dir).byKey)
	if c.byRoot == nil {
		c.byRoot = make(map[string]chunkCount)
	}
	c.byRoot[dir] = chunkCount{total: total, at: now}
	return total
}
