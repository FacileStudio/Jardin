package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// seedPages puts n identical pages on both sides and syncs once, so the base
// records them. Without that first sync a later removal on the server reads as
// a file this machine just created, not as a deletion to propagate.
func seedPages(t *testing.T, c *Client, clientDir, serverDir string, n int) []string {
	t.Helper()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rel := fmt.Sprintf("memory/page%02d.md", i)
		write(t, clientDir, rel, "v1")
		write(t, serverDir, rel, "v1")
		paths = append(paths, rel)
	}
	if _, err := c.Sync(clientDir); err != nil {
		t.Fatal(err)
	}
	return paths
}

// removeFrom deletes pages from one side only. Against the server it is the
// shape of the 2026-08-19 incident, the removals arriving over the wire.
// Against the client it is the other one, a local wipe about to empty the copy
// every other machine pulls from.
func removeFrom(t *testing.T, dir string, paths []string) {
	t.Helper()
	for _, p := range paths {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSyncRefusesToDeleteElevenLocalFiles(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	paths := seedPages(t, c, clientDir, serverDir, 11)
	baseBefore := read(t, clientDir, manifestName)
	removeFrom(t, serverDir, paths)

	res, err := c.Sync(clientDir)
	if res != nil {
		t.Fatalf("expected no result from a refused sync, got %+v", res)
	}
	var refusal *BulkDeleteError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a BulkDeleteError, got %v", err)
	}
	if len(refusal.Local) != 11 || len(refusal.Remote) != 0 {
		t.Fatalf("expected 11 local deletions and no remote ones, got %+v", refusal)
	}
	if !sort.StringsAreSorted(refusal.Local) {
		t.Fatalf("expected the reported paths sorted, got %v", refusal.Local)
	}
	for _, p := range paths {
		if !exists(clientDir, p) {
			t.Fatalf("%s was deleted despite the refusal", p)
		}
	}
	if read(t, clientDir, manifestName) != baseBefore {
		t.Fatal("the base was rewritten despite the refusal, so the next sync would not refuse again")
	}
}

func TestSyncWithAllowBulkDeleteRemovesThem(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	paths := seedPages(t, c, clientDir, serverDir, 11)
	removeFrom(t, serverDir, paths)

	c.AllowBulkDelete = true
	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DeletedLocal) != 11 {
		t.Fatalf("expected 11 local deletions, got %+v", res)
	}
	for _, p := range paths {
		if exists(clientDir, p) {
			t.Fatalf("%s survived a forced sync", p)
		}
	}
}

func TestSyncDeletesTenLocalFilesWithoutAsking(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	paths := seedPages(t, c, clientDir, serverDir, 10)
	removeFrom(t, serverDir, paths)

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatalf("ten deletions is the limit, not past it: %v", err)
	}
	if len(res.DeletedLocal) != 10 {
		t.Fatalf("expected 10 local deletions, got %+v", res)
	}
	for _, p := range paths {
		if exists(clientDir, p) {
			t.Fatalf("%s survived a sync that was under the limit", p)
		}
	}
}

// The other direction, and the one the journal cannot yet undo: a local wipe
// pushed up empties the copy every other machine pulls from. Their own guards
// would then refuse the deletion on the way down, which is the fleet noticing
// after the fact rather than instead of it.
func TestSyncRefusesToDeleteElevenFilesOnTheServer(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	paths := seedPages(t, c, clientDir, serverDir, 11)
	removeFrom(t, clientDir, paths)

	_, err := c.Sync(clientDir)
	var refusal *BulkDeleteError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a BulkDeleteError, got %v", err)
	}
	if len(refusal.Remote) != 11 || len(refusal.Local) != 0 {
		t.Fatalf("expected 11 remote deletions and no local ones, got %+v", refusal)
	}
	for _, p := range paths {
		if !exists(serverDir, p) {
			t.Fatalf("%s was deleted on the server despite the refusal", p)
		}
	}
}

// Six pages disappearing here and six more disappearing there is twelve pages
// gone. The limit measures how much this sync destroys, not how much it
// destroys in whichever direction you happened to look at.
func TestSyncCountsBothDirectionsAgainstOneLimit(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	paths := seedPages(t, c, clientDir, serverDir, 12)
	removeFrom(t, serverDir, paths[:6])
	removeFrom(t, clientDir, paths[6:])

	_, err := c.Sync(clientDir)
	var refusal *BulkDeleteError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a BulkDeleteError, got %v", err)
	}
	if refusal.Total() != 12 || len(refusal.Local) != 6 || len(refusal.Remote) != 6 {
		t.Fatalf("expected six deletions each way, got %+v", refusal)
	}
	for _, p := range paths[:6] {
		if !exists(clientDir, p) {
			t.Fatalf("%s was deleted here despite the refusal", p)
		}
	}
	for _, p := range paths[6:] {
		if !exists(serverDir, p) {
			t.Fatalf("%s was deleted on the server despite the refusal", p)
		}
	}
}
