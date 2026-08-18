package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func reconcileFixture(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(filepath.Join(memDir, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	page := "---\ntitle: T\ntype: tool\n---\n\n### One\nbody one\n\n### Two\nbody two\n"
	for _, name := range []string{"a.md", filepath.Join("tools", "b.md")} {
		if err := os.WriteFile(filepath.Join(memDir, name), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(memDir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(dir, "")
	srv.Semantic = &Semantic{Backend: &fakeBackend{}, Store: &fakeStore{}}
	return srv, dir
}

// TestReconcileQueuesAnEmptyIndex is the gap this closes: the worker only ever
// reacted to sync writes, so turning the feature on against an existing wiki
// indexed nothing until somebody edited a page.
func TestReconcileQueuesAnEmptyIndex(t *testing.T) {
	srv, dir := reconcileFixture(t)
	worker := NewEmbedWorker(srv)
	if worker == nil {
		t.Fatal("want a worker")
	}
	if queued := worker.Reconcile(context.Background(), dir); queued != 2 {
		t.Fatalf("want both markdown pages queued, got %d", queued)
	}
}

// TestReconcileSkipsAnUpToDateIndex keeps a restart from re-embedding a wiki
// that has not changed, which at 1500 chunks is the difference between a no-op
// and a full rebuild.
func TestReconcileSkipsAnUpToDateIndex(t *testing.T) {
	srv, dir := reconcileFixture(t)
	worker := NewEmbedWorker(srv)
	worker.Reconcile(context.Background(), dir)
	worker.flush(context.Background())

	if queued := worker.Reconcile(context.Background(), dir); queued != 0 {
		t.Fatalf("an indexed wiki must queue nothing, got %d", queued)
	}
}

// TestReconcileQueuesAChangedPage proves a page edited while the server was
// down is picked up at start rather than waiting for the next write to it.
func TestReconcileQueuesAChangedPage(t *testing.T) {
	srv, dir := reconcileFixture(t)
	worker := NewEmbedWorker(srv)
	worker.Reconcile(context.Background(), dir)
	worker.flush(context.Background())

	changed := filepath.Join(dir, "memory", "a.md")
	body := "---\ntitle: T\ntype: tool\n---\n\n### One\nbody one edited\n"
	if err := os.WriteFile(changed, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if queued := worker.Reconcile(context.Background(), dir); queued != 1 {
		t.Fatalf("want only the edited page queued, got %d", queued)
	}
}

// TestReconcileIsSafeWithoutSemantic keeps a dormant server from walking the
// wiki at every boot for nothing.
func TestReconcileIsSafeWithoutSemantic(t *testing.T) {
	dir := t.TempDir()
	srv := New(dir, "")
	if worker := NewEmbedWorker(srv); worker != nil {
		t.Fatal("a dormant server must have no worker")
	}
	var nilWorker *EmbedWorker
	if queued := nilWorker.Reconcile(context.Background(), dir); queued != 0 {
		t.Fatalf("a nil worker must queue nothing, got %d", queued)
	}
}
