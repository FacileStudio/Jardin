package server

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

const embedQueueFile = ".embed-queue.json"

// embedTarget is one wiki page waiting to be re-embedded: the tree it lives in
// and its path relative to that tree's memory directory.
type embedTarget struct {
	Root string `json:"root"`
	Path string `json:"path"`
}

func (w *EmbedWorker) wake() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *EmbedWorker) hasPending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending) > 0
}

// take empties the queue and returns it in a stable order, so two runs over the
// same burst embed in the same sequence.
func (w *EmbedWorker) take() []embedTarget {
	w.mu.Lock()
	defer w.mu.Unlock()
	targets := make([]embedTarget, 0, len(w.pending))
	for t := range w.pending {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Root != targets[j].Root {
			return targets[i].Root < targets[j].Root
		}
		return targets[i].Path < targets[j].Path
	})
	w.pending = make(map[embedTarget]bool)
	w.persistLocked()
	return targets
}

func (w *EmbedWorker) requeue(target embedTarget) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending[target] = true
	w.persistLocked()
}

func (w *EmbedWorker) queuePath() string {
	return filepath.Join(w.srv.DataDir, embedQueueFile)
}

// restore reloads a queue left behind by a previous process, so a crash between
// an enqueue and its embedding costs a restart, not a lost page.
func (w *EmbedWorker) restore() {
	data, err := os.ReadFile(w.queuePath())
	if err != nil {
		return
	}
	var targets []embedTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		w.srv.Log.Error("embed worker: corrupt queue",
			slog.String("path", w.queuePath()), slog.Any("error", err))
		return
	}
	for _, t := range targets {
		w.pending[t] = true
	}
}

// persistLocked writes the queue under w.mu, so concurrent enqueues can never
// land on disk out of order and drop an entry one of them had just added.
func (w *EmbedWorker) persistLocked() {
	targets := make([]embedTarget, 0, len(w.pending))
	for t := range w.pending {
		targets = append(targets, t)
	}
	data, err := json.Marshal(targets)
	if err != nil {
		return
	}
	if err := os.WriteFile(w.queuePath(), data, 0o600); err != nil {
		w.srv.Log.Error("embed worker: queue write failed", slog.Any("error", err))
	}
}
