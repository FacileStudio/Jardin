package sync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Jardin/internal/server"
)

func setup(t *testing.T) (*Client, string, string) {
	t.Helper()
	serverDir := t.TempDir()
	clientDir := t.TempDir()
	srv := server.New(serverDir, "")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return NewClient(ts.URL, ""), clientDir, serverDir
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

// establishBase makes both sides identical and records the manifest, simulating
// a clean prior sync.
func establishBase(t *testing.T, c *Client, clientDir, serverDir, rel, content string) {
	t.Helper()
	write(t, clientDir, rel, content)
	write(t, serverDir, rel, content)
	if _, err := c.Sync(clientDir); err != nil {
		t.Fatal(err)
	}
}

// The regression: a fresh local edit must be pushed, never clobbered by the pull.
func TestSyncPushesLocalEditWithoutClobber(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	write(t, clientDir, "rules/a.md", "v2")
	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}

	if read(t, clientDir, "rules/a.md") != "v2" {
		t.Fatal("local edit was clobbered")
	}
	if read(t, serverDir, "rules/a.md") != "v2" {
		t.Fatal("local edit was not pushed to server")
	}
	if len(res.Uploaded) != 1 || res.Uploaded[0] != "rules/a.md" {
		t.Fatalf("expected one upload, got %+v", res)
	}
}

func TestSyncPullsRemoteEdit(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	write(t, serverDir, "rules/a.md", "v2")
	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}

	if read(t, clientDir, "rules/a.md") != "v2" {
		t.Fatal("remote edit was not pulled")
	}
	if len(res.Downloaded) != 1 {
		t.Fatalf("expected one download, got %+v", res)
	}
}

func TestSyncPropagatesLocalDelete(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	if err := os.Remove(filepath.Join(clientDir, "rules", "a.md")); err != nil {
		t.Fatal(err)
	}
	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}

	if exists(serverDir, "rules/a.md") {
		t.Fatal("local delete did not propagate to server")
	}
	if len(res.DeletedRemote) != 1 {
		t.Fatalf("expected one remote delete, got %+v", res)
	}
}

func TestSyncPropagatesRemoteDelete(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	establishBase(t, c, clientDir, serverDir, "rules/a.md", "v1")

	if err := os.Remove(filepath.Join(serverDir, "rules", "a.md")); err != nil {
		t.Fatal(err)
	}
	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}

	if exists(clientDir, "rules/a.md") {
		t.Fatal("remote delete did not propagate to client")
	}
	if len(res.DeletedLocal) != 1 {
		t.Fatalf("expected one local delete, got %+v", res)
	}
}

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

// A brand-new machine (empty local, no manifest) pulls everything.
func TestSyncFreshMachinePullsAll(t *testing.T) {
	c, clientDir, serverDir := setup(t)
	write(t, serverDir, "rules/a.md", "v1")
	write(t, serverDir, "memory/index.md", "# Index")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Downloaded) != 2 {
		t.Fatalf("expected two downloads, got %+v", res)
	}
	if read(t, clientDir, "rules/a.md") != "v1" {
		t.Fatal("fresh pull missing content")
	}
}

// A Client with Space set must send space_id on every sync request; without it,
// no space_id is sent (the server then serves the common tree). Tested against
// a stub server because the real server's space support ships separately.
func TestClientSendsSpaceID(t *testing.T) {
	queries := map[string]string{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries[r.Method+" "+r.URL.Path] = r.URL.RawQuery
		switch {
		case r.URL.Path == "/api/sync/tree":
			w.Write([]byte("[]"))
		case r.Method == http.MethodGet:
			w.Write([]byte("data"))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer stub.Close()

	c := NewClient(stub.URL, "")
	c.Space = "space-42"

	if _, err := c.Tree(); err != nil {
		t.Fatal(err)
	}
	if err := c.Upload("rules/a.md", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Download("rules/a.md"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("rules/a.md"); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{
		"GET /api/sync/tree",
		"PUT /api/sync/files/rules/a.md",
		"GET /api/sync/files/rules/a.md",
		"DELETE /api/sync/files/rules/a.md",
	} {
		if got, want := queries[key], "space_id=space-42"; got != want {
			t.Fatalf("%s: query = %q, want %q", key, got, want)
		}
	}

	c.Space = ""
	if _, err := c.Tree(); err != nil {
		t.Fatal(err)
	}
	if q := queries["GET /api/sync/tree"]; q != "" {
		t.Fatalf("Space unset should send no query, got %q", q)
	}
}

// Machine-local files (logs, tokens, dotfiles, conflict backups) never sync.
func TestSyncSkipsMachineLocalFiles(t *testing.T) {
	c, clientDir, _ := setup(t)
	write(t, clientDir, "daemon.log", "local log noise")
	write(t, clientDir, "tokens.json", "{}")
	write(t, clientDir, "rules/a.md.conflict", "backup")
	write(t, clientDir, "rules/a.md", "v1")

	res, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Uploaded) != 1 || res.Uploaded[0] != "rules/a.md" {
		t.Fatalf("expected only rules/a.md to upload, got %+v", res)
	}
}

// Run artifacts capture step output, which is exactly where a leaked secret
// would land, so they stay on the machine that produced them. Flow files sync.
func TestSyncSkipsRunArtifacts(t *testing.T) {
	for rel, want := range map[string]bool{
		"runs/x/y.json": true,
		"flows/x.yml":   false,
	} {
		if got := syncSkip(rel); got != want {
			t.Fatalf("syncSkip(%q) = %v, want %v", rel, got, want)
		}
	}
}
