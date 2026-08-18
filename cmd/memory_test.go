package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
)

const memoryFixture = "---\ntitle: Ranking\n---\n\n### bm25 replaced the strict AND\n" +
	"Strict AND collapsed on reformulated queries.\n"

func memoryFixtureDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "memory", "ranking.md")
	if err := os.WriteFile(path, []byte(memoryFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("DATA_DIR", dir)
	memorySearchLimit, memorySearchLocal, memorySearchVerbose = 20, false, false
	t.Cleanup(func() { memorySearchLimit, memorySearchLocal, memorySearchVerbose = 20, false, false })
}

func TestSearchMemoryFallsBackWhenServerIsUnreachable(t *testing.T) {
	memoryFixtureDir(t)
	t.Setenv(config.VectorSearchEnv, "true")
	t.Setenv(config.URLEnv, "http://127.0.0.1:1")
	t.Setenv(config.TokenEnv, "tok")

	results, err := searchMemory("bm25 ranking")
	if err != nil {
		t.Fatalf("searchMemory returned an error instead of local results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("searchMemory returned no results, want the local index to answer")
	}
	if results[0].Path != "ranking.md" {
		t.Errorf("results[0].Path = %q, want ranking.md", results[0].Path)
	}
}

func TestSearchMemoryLocalFlagSkipsTheServer(t *testing.T) {
	memoryFixtureDir(t)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	t.Setenv(config.VectorSearchEnv, "true")
	t.Setenv(config.URLEnv, srv.URL)
	t.Setenv(config.TokenEnv, "tok")
	memorySearchLocal = true

	results, err := searchMemory("bm25 ranking")
	if err != nil {
		t.Fatalf("searchMemory: %v", err)
	}
	if called {
		t.Errorf("the server was called although --local was set")
	}
	if len(results) == 0 {
		t.Fatalf("searchMemory returned no results, want the local index to answer")
	}
}

func TestSearchMemoryPrefersTheServer(t *testing.T) {
	memoryFixtureDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"path":"remote.md","line":3,"excerpt":"from the server"}]}`))
	}))
	defer srv.Close()
	t.Setenv(config.VectorSearchEnv, "true")
	t.Setenv(config.URLEnv, srv.URL)
	t.Setenv(config.TokenEnv, "tok")

	results, err := searchMemory("bm25 ranking")
	if err != nil {
		t.Fatalf("searchMemory: %v", err)
	}
	if len(results) != 1 || results[0].Path != "remote.md" {
		t.Fatalf("results = %+v, want the server's single hit", results)
	}
}
