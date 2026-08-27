package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/reports"
)

func TestReportsAPI(t *testing.T) {
	dataDir := t.TempDir()
	srv := New(dataDir, "password")
	handler := srv.Handler()
	token := loginAs(t, handler, "password", "lucy")

	// Seed a report
	src := filepath.Join(t.TempDir(), "audit.html")
	if err := os.WriteFile(src, []byte("<html><head><title>Audit 2026</title></head><body><h1>Audit</h1></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := reports.Add(dataDir, reports.Request{
		Source:  src,
		Machine: "lucy",
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	// 1. List reports
	req := httptest.NewRequest("GET", "/api/reports", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/reports: got %d, want 200", w.Code)
	}
	var list []ReportSummary
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "audit-2026" {
		t.Fatalf("expected audit-2026 in list, got %+v", list)
	}

	// 2. Get report detail
	req = httptest.NewRequest("GET", "/api/reports/audit-2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/reports/audit-2026: got %d, want 200", w.Code)
	}
	var detail ReportDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != "audit-2026" || detail.Title != "Audit 2026" || detail.Machine != "lucy" {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	// 3. Delete report
	req = httptest.NewRequest("DELETE", "/api/reports/audit-2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/reports/audit-2026: got %d, want 204", w.Code)
	}

	// 4. Confirm deleted
	req = httptest.NewRequest("GET", "/api/reports/audit-2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/reports/audit-2026 after delete: got %d, want 404", w.Code)
	}
}
