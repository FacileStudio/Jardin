package sync

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
