package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/artifacts"
)

func seedArtifact(t *testing.T, dataDir string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "audit.html")
	if err := os.WriteFile(src, []byte("<html><head><title>Audit 2026</title></head><body><h1>Audit</h1></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	art, err := artifacts.Add(dataDir, artifacts.Request{
		Source:  src,
		Machine: "lucy",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return art.ID
}

func TestArtifactsListAPI(t *testing.T) {
	dataDir := t.TempDir()
	srv := New(dataDir, "password")
	handler := srv.Handler()
	token := loginAs(t, handler, "password", "lucy")
	id := seedArtifact(t, dataDir)

	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/artifacts: got %d, want 200", w.Code)
	}
	var list []ArtifactSummary
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("expected %s in list, got %+v", id, list)
	}
	if list[0].Format != "html" {
		t.Fatalf("expected format html, got %s", list[0].Format)
	}
}

func TestArtifactsGetAPI(t *testing.T) {
	dataDir := t.TempDir()
	srv := New(dataDir, "password")
	handler := srv.Handler()
	token := loginAs(t, handler, "password", "lucy")
	id := seedArtifact(t, dataDir)

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/artifacts/%s: got %d, want 200", id, w.Code)
	}
	var detail ArtifactDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != id || detail.Title != "Audit 2026" || detail.Machine != "lucy" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestArtifactsDeleteAPI(t *testing.T) {
	dataDir := t.TempDir()
	srv := New(dataDir, "password")
	handler := srv.Handler()
	token := loginAs(t, handler, "password", "lucy")
	id := seedArtifact(t, dataDir)

	req := httptest.NewRequest("DELETE", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/artifacts/%s: got %d, want 204", id, w.Code)
	}

	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/artifacts/%s after delete: got %d, want 404", id, w.Code)
	}
}
