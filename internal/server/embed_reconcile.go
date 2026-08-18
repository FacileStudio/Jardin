package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/FacileStudio/Jardin/internal/memory"
)

// Reconcile enqueues every page of a tree whose chunks are not already in the
// index, and reports how many it queued. The worker is otherwise driven by sync
// writes, so without this a store that has never seen a write stays empty
// forever: switching the feature on would index nothing until somebody happened
// to edit a page.
//
// It also covers the two other ways the index falls behind — a model change,
// which empties the store by design, and a stretch of downtime during which
// pages changed. The index is derived, so it is allowed to rebuild itself
// rather than wait for a human to press something.
//
// Only the tree at root is walked. A space is reconciled when its own root is
// passed, not implicitly.
func (w *EmbedWorker) Reconcile(ctx context.Context, root string) int {
	if w == nil || w.srv.Semantic == nil {
		return 0
	}
	hashes := w.store().Hashes()
	scope := w.srv.treeScope(root)
	queued := 0
	err := filepath.Walk(filepath.Join(root, "memory"), func(path string, info os.FileInfo, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if w.pageNeedsIndexing(root, path, scope, hashes) {
			w.Enqueue(root, path)
			queued++
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		w.log().Warn("embed reconcile walk failed", slog.Any("error", err))
	}
	return queued
}

// reconcileAll reconciles the common tree and every space. A space's existing
// pages are as unindexed as the common tree's were, and nothing else would ever
// queue them: the worker reacts to writes, and a space that is not being edited
// produces none.
func (w *EmbedWorker) reconcileAll(ctx context.Context) int {
	if w == nil {
		return 0
	}
	queued := w.Reconcile(ctx, w.srv.DataDir)
	for id := range w.srv.loadSpaces() {
		queued += w.Reconcile(ctx, filepath.Join(w.srv.spacesPath(), id))
	}
	return queued
}

// pageNeedsIndexing reports whether any chunk of a page is missing from the
// index or has changed since it was embedded. It derives the chunk path through
// embedTargetFor, the same function the sync path uses, so a reconcile can
// never compute a key the worker would not.
func (w *EmbedWorker) pageNeedsIndexing(root, path, scope string, hashes map[string]string) bool {
	target, ok := embedTargetFor(root, path)
	if !ok {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, chunk := range memory.Chunks(target.Path, string(data)) {
		if hashes[scopedKey(scope, memory.ChunkKey(chunk))] != memory.ChunkHash(chunk) {
			return true
		}
	}
	return false
}

func (w *EmbedWorker) store() memory.Store {
	return w.srv.Semantic.Store
}

func (w *EmbedWorker) log() *slog.Logger {
	return w.srv.Log
}
