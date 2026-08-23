package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A genuine edit-vs-edit conflict must converge without losing either version.
// Both edits survive: the winner under its own name, the loser mirrored under
// .conflicts/ where it never syncs and never shows up in a directory read of
// memory/, and a second sync with no further edits converges to a no-op.
func TestSyncConflictKeepsBothVersions(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	write(t, clientDir, "rules/a.md", "local-edit")
	write(t, serverDir, "rules/a.md", "server-edit")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", res)
	}

	winner := read(t, clientDir, "rules/a.md")
	if !exists(clientDir, ".conflicts/rules/a.md") {
		t.Fatal("conflict backup was not written")
	}
	loser := read(t, clientDir, ".conflicts/rules/a.md")

	both := winner + "|" + loser
	if !strings.Contains(both, "local-edit") || !strings.Contains(both, "server-edit") {
		t.Fatalf("a version was lost: winner=%q loser=%q", winner, loser)
	}

	if exists(serverDir, ".conflicts/rules/a.md") {
		t.Fatal("the conflict copy leaked to the server")
	}

	res2, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Total() != 0 {
		t.Fatalf("sync did not converge, second pass did %+v", res2)
	}
}

// usage/<machine>/ has one writer, so a both-sides difference is that writer
// racing its own status line: the fresher copy wins outright, with no backup and
// no conflict, and the next pass has nothing left to do.
func TestSyncSingleWriterUsagePathTakesFresherCopy(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "usage/ruche/current.json", `{"used":1}`)

	write(t, serverDir, "usage/ruche/current.json", `{"used":2}`)
	write(t, clientDir, "usage/ruche/current.json", `{"used":3}`)

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("single-writer path reported as a conflict: %+v", res.Conflicts)
	}
	if exists(clientDir, ".conflicts/usage/ruche/current.json") {
		t.Fatal("single-writer path kept a conflict backup")
	}
	if got := read(t, clientDir, "usage/ruche/current.json"); got != `{"used":3}` {
		t.Fatalf("fresher local copy did not win, got %q", got)
	}
	if got := read(t, serverDir, "usage/ruche/current.json"); got != `{"used":3}` {
		t.Fatalf("the winner was not pushed, got %q", got)
	}

	res2, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Total() != 0 {
		t.Fatalf("sync did not converge, a second pass still did %+v", res2)
	}
}

// A backup left by the old behaviour is this machine's own stale telemetry and
// nothing else deletes it, so doctor would stay red forever. It is written in
// the pre-.conflicts/ layout on purpose: machines are still carrying those, and
// the prune has to reach both.
//
// The sync that clears it must be an ordinary one. Once conflicts on these paths
// are prevented the resolve path is never reached again, so cleaning up only
// there would heal nothing on a machine that is already quiet — which is every
// machine carrying one of these files.
func TestSyncClearsAStaleConflictBackupWithNothingElseToDo(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "usage/ruche/current.json", `{"used":1}`)
	write(t, clientDir, "usage/ruche/current.json.conflict", `{"used":0}`)

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total() != 0 {
		t.Fatalf("expected a no-op sync, got %+v", res)
	}
	if exists(clientDir, "usage/ruche/current.json.conflict") {
		t.Fatal("a stale backup survived a sync that had nothing else to do")
	}
}

// The prune is scoped like the exemption itself: a backup on a path with no
// single owner is a real second version and must survive.
func TestSyncKeepsAConflictBackupOnASharedPath(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "usage/shared.json", "v1")
	write(t, clientDir, "usage/shared.json.conflict", "the other version")

	if _, err := c.Sync(clientDir); err != nil {
		t.Fatal(err)
	}
	if !exists(clientDir, "usage/shared.json.conflict") {
		t.Fatal("a backup on a shared path was pruned")
	}
}

// The exemption is scoped to a machine directory: usage/<file> has no single
// owner, so it still conflicts like anything else.
func TestSyncUsageRootStillConflicts(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "usage/shared.json", "v1")

	write(t, clientDir, "usage/shared.json", "local-edit")
	write(t, serverDir, "usage/shared.json", "server-edit")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", res.Conflicts)
	}
	if !exists(clientDir, ".conflicts/usage/shared.json") {
		t.Fatal("conflict backup was not written")
	}
}

// touch forces a file's modification time so a conflict has a predictable
// winner: localWins compares mod times before falling back to the checksum.
func touch(t *testing.T, dir, rel string, when time.Time) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.Chtimes(full, when, when); err != nil {
		t.Fatal(err)
	}
}

// Content beats a deletion. Deleting a file locally while someone edited it on
// the server must restore the server's copy, not confirm the delete -- the edit
// is work that exists nowhere else, and the delete can be repeated for free.
func TestSyncDeletedLocallyButEditedOnServerKeepsTheServerCopy(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	if err := os.Remove(filepath.Join(clientDir, "rules", "a.md")); err != nil {
		t.Fatal(err)
	}
	write(t, serverDir, "rules/a.md", "server-edit")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, clientDir, "rules/a.md"); got != "server-edit" {
		t.Fatalf("server edit was lost to a local delete, got %q", got)
	}
	if len(res.Downloaded) != 1 || res.Downloaded[0] != "rules/a.md" {
		t.Fatalf("expected one download, got %+v", res)
	}
	if len(res.Conflicts) != 1 || !strings.Contains(res.Conflicts[0], "deleted locally") {
		t.Fatalf("conflict not reported as a local delete: %+v", res.Conflicts)
	}
	if exists(clientDir, ".conflicts/rules/a.md") {
		t.Fatal("a backup was written for a delete-vs-edit, which has only one version")
	}
}

// The mirror case: the server dropped a file this machine had edited. The local
// edit wins and is pushed back up.
func TestSyncDeletedOnServerButEditedLocallyKeepsTheLocalCopy(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	if err := os.Remove(filepath.Join(serverDir, "rules", "a.md")); err != nil {
		t.Fatal(err)
	}
	write(t, clientDir, "rules/a.md", "local-edit")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, clientDir, "rules/a.md"); got != "local-edit" {
		t.Fatalf("local edit was lost to a server delete, got %q", got)
	}
	if got := read(t, serverDir, "rules/a.md"); got != "local-edit" {
		t.Fatalf("local edit was not restored to the server, got %q", got)
	}
	if len(res.Uploaded) != 1 {
		t.Fatalf("expected one upload, got %+v", res)
	}
	if len(res.Conflicts) != 1 || !strings.Contains(res.Conflicts[0], "deleted on server") {
		t.Fatalf("conflict not reported as a server delete: %+v", res.Conflicts)
	}
}

// The losing side of an edit-vs-edit is kept whichever side loses.
// The existing conflict test covers the local-wins direction; this pins the
// other, where the local file is the one moved aside.
func TestSyncRemoteWinsKeepsTheLocalCopyAsBackup(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	write(t, clientDir, "rules/a.md", "local-edit")
	write(t, serverDir, "rules/a.md", "server-edit")
	touch(t, clientDir, "rules/a.md", time.Now().Add(-time.Hour))
	touch(t, serverDir, "rules/a.md", time.Now().Add(time.Hour))

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, clientDir, "rules/a.md"); got != "server-edit" {
		t.Fatalf("the fresher server copy did not win, got %q", got)
	}
	if got := read(t, clientDir, ".conflicts/rules/a.md"); got != "local-edit" {
		t.Fatalf("the losing local edit was not kept as a backup, got %q", got)
	}
	if len(res.Downloaded) != 1 {
		t.Fatalf("expected one download, got %+v", res)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", res.Conflicts)
	}
}

// The single-writer exemption has to work in both directions too: a fresher
// server copy is taken without a backup and without reporting a conflict.
func TestSyncSingleWriterTakesFresherServerCopy(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "usage/ruche/current.json", `{"used":1}`)

	write(t, clientDir, "usage/ruche/current.json", `{"used":2}`)
	write(t, serverDir, "usage/ruche/current.json", `{"used":3}`)
	touch(t, clientDir, "usage/ruche/current.json", time.Now().Add(-time.Hour))
	touch(t, serverDir, "usage/ruche/current.json", time.Now().Add(time.Hour))

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, clientDir, "usage/ruche/current.json"); got != `{"used":3}` {
		t.Fatalf("fresher server copy did not win, got %q", got)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("single-writer path reported a conflict: %+v", res.Conflicts)
	}
	if exists(clientDir, ".conflicts/usage/ruche/current.json") {
		t.Fatal("single-writer path kept a backup")
	}
	if len(res.Downloaded) != 1 {
		t.Fatalf("expected one download, got %+v", res)
	}
}
