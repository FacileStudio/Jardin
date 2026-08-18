package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/FacileStudio/Jardin/internal/memory"
)

const (
	embedDebounce  = 2 * time.Second
	embedTimeout   = 2 * time.Minute
	scopeSeparator = "|"
)

// EmbedWorker re-embeds changed wiki pages away from the request path. Sync
// handlers enqueue and return, so a sync writing twelve files never waits on
// twelve embeddings — blocking a request on a model is the failure mode this
// whole type exists to prevent. The queue is written to disk on every enqueue,
// so a crash mid-burst loses no work.
type EmbedWorker struct {
	srv      *Server
	mu       sync.Mutex
	pending  map[embedTarget]bool
	kick     chan struct{}
	debounce time.Duration

	active    bool
	processed int
	startedAt time.Time
	updatedAt time.Time
	lastError string
}

// NewEmbedWorker builds the worker for a server's semantic half and wires it
// in. It returns nil when no embedding backend is configured, and every method
// tolerates a nil receiver, so a caller can build and run it unconditionally.
func NewEmbedWorker(srv *Server) *EmbedWorker {
	if srv.Semantic == nil {
		return nil
	}
	w := &EmbedWorker{
		srv:      srv,
		pending:  make(map[embedTarget]bool),
		kick:     make(chan struct{}, 1),
		debounce: embedDebounce,
	}
	w.restore()
	srv.Semantic.worker = w
	return w
}

// enqueueEmbed queues a changed file for re-embedding and returns at once.
func (s *Server) enqueueEmbed(root, full string) {
	if s.Semantic == nil {
		return
	}
	s.Semantic.worker.Enqueue(root, full)
}

// treeScope names the tree a root belongs to. Chunk paths are relative to their
// own tree's memory directory, so without this prefix the common wiki and a
// space would claim the same key in one shared index.
func (s *Server) treeScope(root string) string {
	rel, err := filepath.Rel(s.DataDir, root)
	if err != nil {
		return root
	}
	return rel
}

func scopedKey(scope, key string) string {
	return scope + scopeSeparator + key
}

func unscope(scope, key string) (string, bool) {
	prefix := scope + scopeSeparator
	if !strings.HasPrefix(key, prefix) {
		return "", false
	}
	return strings.TrimPrefix(key, prefix), true
}

// Enqueue records a changed path. It never blocks: the queue is a map and the
// kick channel is buffered and coalescing, so a burst of writes costs a burst
// of map writes and at most one wakeup.
func (w *EmbedWorker) Enqueue(root, full string) {
	if w == nil {
		return
	}
	target, ok := embedTargetFor(root, full)
	if !ok {
		return
	}
	w.mu.Lock()
	w.pending[target] = true
	w.persistLocked()
	w.mu.Unlock()
	w.wake()
}

// Run drains the queue until the context ends. Changes are debounced and
// coalesced by path, so a sync burst embeds each page once rather than once per
// write.
func (w *EmbedWorker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	if queued := w.Reconcile(ctx, w.srv.DataDir); queued > 0 {
		w.log().Info("embed reconcile queued pages", slog.Int("pages", queued))
	}
	if w.hasPending() {
		w.wake()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.kick:
		}
		if !w.settle(ctx) {
			return
		}
		w.flush(ctx)
	}
}

// settle waits out the debounce window, restarting it on every further change,
// and reports whether the worker should keep running.
func (w *EmbedWorker) settle(ctx context.Context) bool {
	timer := time.NewTimer(w.debounce)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-w.kick:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(w.debounce)
		case <-timer.C:
			return true
		}
	}
}

// flush embeds every queued page. A failure is logged and requeued without a
// wakeup, so work survives a model outage without spinning against it: the next
// sync picks it back up.
func (w *EmbedWorker) flush(ctx context.Context) {
	targets := w.take()
	if len(targets) == 0 {
		return
	}
	w.beginPass(time.Now().UTC())
	defer func() { w.endPass(time.Now().UTC()) }()
	for _, target := range targets {
		if err := w.process(ctx, target); err != nil {
			w.srv.Log.Error("embed worker: page failed",
				slog.String("path", target.Path), slog.Any("error", err))
			w.recordError(err, time.Now().UTC())
			w.requeue(target)
		}
	}
}

// process re-embeds one page. Only the chunks whose content hash changed reach
// the model; a page whose blocks moved or disappeared is rebuilt whole, because
// the store can drop vectors by path but not by key.
func (w *EmbedWorker) process(ctx context.Context, target embedTarget) error {
	semantic := w.srv.Semantic
	if semantic == nil || semantic.Backend == nil || semantic.Store == nil {
		return nil
	}
	scope := w.srv.treeScope(target.Root)
	data, err := os.ReadFile(filepath.Join(target.Root, "memory", target.Path))
	if err != nil {
		if os.IsNotExist(err) {
			return semantic.Store.DeletePaths([]string{scopedKey(scope, target.Path)})
		}
		return err
	}

	chunks := memory.Chunks(target.Path, string(data))
	changed, rebuild := changedChunks(chunks, semantic.Store.Hashes(), scope, target.Path)
	if rebuild {
		if err := semantic.Store.DeletePaths([]string{scopedKey(scope, target.Path)}); err != nil {
			return err
		}
		changed = chunks
	}
	if len(changed) == 0 {
		return nil
	}
	return w.embedAndUpsert(ctx, scope, changed)
}

// changedChunks selects what still has to be embedded and reports whether the
// page must be rebuilt from scratch. A stored key the page no longer has means
// its blocks moved, and a stale vector pointing at content that is gone is
// worse than paying for the page again.
func changedChunks(chunks []memory.Chunk, hashes map[string]string, scope, path string) ([]memory.Chunk, bool) {
	live := make(map[string]bool, len(chunks))
	var changed []memory.Chunk
	for _, c := range chunks {
		key := scopedKey(scope, memory.ChunkKey(c))
		live[key] = true
		if hashes[key] != memory.ChunkHash(c) {
			changed = append(changed, c)
		}
	}
	prefix := scopedKey(scope, path) + "#"
	for key := range hashes {
		if strings.HasPrefix(key, prefix) && !live[key] {
			return nil, true
		}
	}
	return changed, false
}

func (w *EmbedWorker) embedAndUpsert(ctx context.Context, scope string, chunks []memory.Chunk) error {
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		texts = append(texts, c.Text())
	}
	embedCtx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	vectors, err := w.srv.Semantic.Backend.Embed(embedCtx, texts)
	if err != nil {
		return err
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embed worker: model returned %d vectors for %d chunks", len(vectors), len(chunks))
	}
	entries := make([]memory.Entry, 0, len(chunks))
	for i, c := range chunks {
		entries = append(entries, memory.Entry{
			Key:     scopedKey(scope, memory.ChunkKey(c)),
			Path:    scopedKey(scope, c.Path),
			Heading: c.Heading,
			Line:    c.Line,
			Hash:    memory.ChunkHash(c),
			Vector:  vectors[i],
		})
	}
	if err := w.srv.Semantic.Store.Upsert(entries); err != nil {
		return err
	}
	w.recordProgress(len(entries), time.Now().UTC())
	return nil
}

// embedTargetFor keeps the queue to wiki pages: anything outside a tree's
// memory directory, and anything that is not markdown, is not embeddable.
func embedTargetFor(root, full string) (embedTarget, bool) {
	rel, err := filepath.Rel(filepath.Join(root, "memory"), full)
	if err != nil || strings.HasPrefix(rel, "..") || !strings.HasSuffix(rel, ".md") {
		return embedTarget{}, false
	}
	return embedTarget{Root: root, Path: filepath.ToSlash(rel)}, true
}
