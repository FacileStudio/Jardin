package server

import "time"

// EmbedStatus is a snapshot of what the worker is doing. It is a copy taken
// under the worker's lock rather than a view onto live fields, so a reader
// polling every two seconds never observes a half-updated pass.
type EmbedStatus struct {
	Indexing  bool
	Pending   int
	Processed int
	StartedAt time.Time
	UpdatedAt time.Time
	LastError string
}

// Status reports the current pass. A nil worker is the dormant configuration
// and answers with the zero snapshot, so a caller never has to ask whether
// embedding is configured before asking how it is going.
func (w *EmbedWorker) Status() EmbedStatus {
	if w == nil {
		return EmbedStatus{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return EmbedStatus{
		Indexing:  w.active || len(w.pending) > 0,
		Pending:   len(w.pending),
		Processed: w.processed,
		StartedAt: w.startedAt,
		UpdatedAt: w.updatedAt,
		LastError: w.lastError,
	}
}

// beginPass starts the clock a rate is measured against. Only a pass that
// follows an idle worker resets it: a full index drains as one pass, and
// restarting the count mid-drain would make every estimate the estimate for the
// last few seconds instead of for the run.
func (w *EmbedWorker) beginPass(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.active {
		w.startedAt = now
		w.processed = 0
	}
	w.active = true
	w.updatedAt = now
	w.lastError = ""
}

func (w *EmbedWorker) endPass(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.active = false
	w.updatedAt = now
}

// recordProgress counts chunks that reached the store, not pages, because a
// page is worth anywhere from one chunk to fifty and a per-page rate would
// predict the wrong finish time by that ratio.
func (w *EmbedWorker) recordProgress(chunks int, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.processed += chunks
	w.updatedAt = now
}

// recordError keeps the last failure visible until the next pass clears it, so
// a model outage shows up as a stalled index with a reason rather than as an
// index that silently stopped moving.
func (w *EmbedWorker) recordError(err error, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastError = err.Error()
	w.updatedAt = now
}
