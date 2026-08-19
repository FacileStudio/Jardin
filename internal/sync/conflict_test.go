package sync

import (
	"strings"
	"testing"
)

// A genuine edit-vs-edit conflict must converge without losing either version.
// Both edits survive: the winner under its own name, the loser as a sibling
// .conflict file that never syncs to the server, and a second sync with no
// further edits converges to a no-op.
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
	if !exists(clientDir, "rules/a.md.conflict") {
		t.Fatal("conflict backup was not written")
	}
	loser := read(t, clientDir, "rules/a.md.conflict")

	both := winner + "|" + loser
	if !strings.Contains(both, "local-edit") || !strings.Contains(both, "server-edit") {
		t.Fatalf("a version was lost: winner=%q loser=%q", winner, loser)
	}

	if exists(serverDir, "rules/a.md.conflict") {
		t.Fatal(".conflict file leaked to the server")
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
	if exists(clientDir, "usage/ruche/current.json.conflict") {
		t.Fatal("single-writer path kept a .conflict backup")
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
// nothing else deletes it, so doctor would stay red forever.
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
	if !exists(clientDir, "usage/shared.json.conflict") {
		t.Fatal("conflict backup was not written")
	}
}
