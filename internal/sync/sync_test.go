package sync

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Ruche/internal/server"
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

	// Conflict backups never sync to the server.
	if exists(serverDir, "rules/a.md.conflict") {
		t.Fatal(".conflict file leaked to the server")
	}

	// Converged: a second sync with no edits is a no-op.
	res2, err := c.Sync(clientDir)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Total() != 0 {
		t.Fatalf("sync did not converge, second pass did %+v", res2)
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
