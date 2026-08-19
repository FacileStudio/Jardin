package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
)

func embedWorkerFor(t *testing.T, backend *fakeBackend, store *fakeStore) (*Server, *EmbedWorker) {
	t.Helper()
	s := wikiServer(t)
	s.Semantic = &Semantic{Backend: backend, Store: store}
	w := NewEmbedWorker(s)
	if w == nil {
		t.Fatal("worker should exist when the semantic half is configured")
	}
	w.debounce = 5 * time.Millisecond
	return s, w
}

// TestSyncPutDoesNotWaitOnTheModel is the reason the worker exists: the request
// path must not block on the model.
//
// The bound is derived from the backend's own delay rather than picked. A
// handler that waited even once could not come back in less than one call, so
// anything well under that proves it did not. The previous bound was 100ms
// against a 500ms backend — five times tighter than the property needs — and it
// failed a pull request at 474ms, which measured how busy the runner was and
// nothing about this code.
func TestSyncPutDoesNotWaitOnTheModel(t *testing.T) {
	const backendDelay = 2 * time.Second
	backend := &fakeBackend{delay: backendDelay}
	s, w := embedWorkerFor(t, backend, &fakeStore{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	h := s.Handler()
	start := time.Now()
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		req := httptest.NewRequest("PUT", "/api/sync/files/memory/"+name+".md",
			bytes.NewReader([]byte(wikiPage)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("put %s: status %d", name, rec.Code)
		}
	}
	elapsed := time.Since(start)

	if elapsed >= backendDelay/2 {
		t.Fatalf("six syncs took %v against a %v backend: the request path is waiting on the model",
			elapsed, backendDelay)
	}
	if got := len(backend.embedded()); got != 0 {
		t.Fatalf("model called %d times before the debounce elapsed", got)
	}
}

// TestWorkerCoalescesABurst checks the debounce: repeated writes to one page
// during a sync burst cost one pass, not one per write.
func TestWorkerCoalescesABurst(t *testing.T) {
	backend := &fakeBackend{}
	s, w := embedWorkerFor(t, backend, &fakeStore{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	page := filepath.Join(s.DataDir, "memory", "tools", "sync.md")
	for i := 0; i < 5; i++ {
		w.Enqueue(s.DataDir, page)
	}
	waitFor(t, func() bool { return len(backend.embedded()) > 0 })
	time.Sleep(50 * time.Millisecond)

	chunks := memory.Chunks("tools/sync.md", wikiPage)
	if got := len(backend.embedded()); got != len(chunks) {
		t.Fatalf("embedded %d texts, want %d — the burst was not coalesced", got, len(chunks))
	}
}

// TestWorkerEmbedsOnlyChangedChunks is the incremental-index guarantee: a page
// with one edited block costs one embedding, not a whole page.
func TestWorkerEmbedsOnlyChangedChunks(t *testing.T) {
	backend := &fakeBackend{}
	store := &fakeStore{hashes: map[string]string{}}
	s, w := embedWorkerFor(t, backend, store)

	chunks := memory.Chunks("tools/sync.md", wikiPage)
	if len(chunks) < 3 {
		t.Fatalf("fixture yielded %d chunks, expected the preamble plus two findings", len(chunks))
	}
	changed := chunks[1]
	for _, c := range chunks {
		hash := memory.ChunkHash(c)
		if c.Line == changed.Line {
			hash = "stale"
		}
		store.hashes[scopedKey(".", memory.ChunkKey(c))] = hash
	}

	if err := w.process(context.Background(), embedTarget{Root: s.DataDir, Path: "tools/sync.md"}); err != nil {
		t.Fatalf("process: %v", err)
	}

	seen := backend.embedded()
	if len(seen) != 1 || seen[0] != changed.Text() {
		t.Fatalf("embedded %d texts %q, want only the changed chunk", len(seen), seen)
	}
	upserts := store.upserts()
	if len(upserts) != 1 || upserts[0].Key != scopedKey(".", memory.ChunkKey(changed)) {
		t.Fatalf("upserted %+v, want only the changed chunk", upserts)
	}
	if upserts[0].Hash != memory.ChunkHash(changed) {
		t.Error("upserted entry does not carry the new content hash")
	}
}

// TestWorkerDropsADeletedPage checks the other direction: a removed page leaves
// no vectors behind to be retrieved.
func TestWorkerDropsADeletedPage(t *testing.T) {
	backend := &fakeBackend{}
	store := &fakeStore{hashes: map[string]string{}}
	s, w := embedWorkerFor(t, backend, store)
	if err := os.Remove(filepath.Join(s.DataDir, "memory", "tools", "sync.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if err := w.process(context.Background(), embedTarget{Root: s.DataDir, Path: "tools/sync.md"}); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != scopedKey(".", "tools/sync.md") {
		t.Fatalf("deleted %v, want the page's scoped path", store.deleted)
	}
	if len(backend.embedded()) != 0 {
		t.Error("a deleted page must not be embedded")
	}
}

// TestWorkerQueueSurvivesRestart covers the crash case: work enqueued by one
// process is still there for the next one.
func TestWorkerQueueSurvivesRestart(t *testing.T) {
	s, w := embedWorkerFor(t, &fakeBackend{}, &fakeStore{})
	w.Enqueue(s.DataDir, filepath.Join(s.DataDir, "memory", "tools", "sync.md"))

	revived := NewEmbedWorker(s)
	if !revived.hasPending() {
		t.Fatal("a queue written before the crash must be reloaded")
	}
	want := embedTarget{Root: s.DataDir, Path: "tools/sync.md"}
	if got := revived.take(); len(got) != 1 || got[0] != want {
		t.Fatalf("restored %+v, want %+v", got, want)
	}
}

// TestEnqueueIgnoresNonWikiWrites keeps the queue to what can be embedded:
// rules, skills and anything outside a memory directory are not wiki pages.
func TestEnqueueIgnoresNonWikiWrites(t *testing.T) {
	s, w := embedWorkerFor(t, &fakeBackend{}, &fakeStore{})
	for _, path := range []string{"rules/style.md", "memory/notes.txt", "../escape.md"} {
		w.Enqueue(s.DataDir, filepath.Join(s.DataDir, path))
	}
	if w.hasPending() {
		t.Fatalf("queued a non-wiki write: %+v", w.take())
	}
}

func TestEnqueueOnDormantServerIsANoOp(t *testing.T) {
	s := wikiServer(t)
	s.enqueueEmbed(s.DataDir, filepath.Join(s.DataDir, "memory", "tools", "sync.md"))
	if NewEmbedWorker(s) != nil {
		t.Fatal("no semantic half configured, so there is no worker to build")
	}
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the worker")
}
