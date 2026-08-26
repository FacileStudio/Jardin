package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// A synced .html must never come back as a document. The tree carries
// agent-authored HTML in reports/, and this origin's localStorage holds the
// bearer token that mints API tokens and writes rules/, so a response the
// browser is willing to render is one prompt injection away from a machine
// takeover. Header-only auth is what stops that today; the content type is
// what stops it if anything ever renders what this endpoint returns.
func TestSyncFilesNeverServeHTMLAsADocument(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "lucy")

	page := []byte("<html><body><script>steal()</script></body></html>")
	if w := doReq(t, h, rawCall{"PUT", "/api/sync/files/reports/x.html", token, page}); w.Code != http.StatusNoContent {
		t.Fatalf("put: got %d, want 204", w.Code)
	}

	w := doReq(t, h, rawCall{Method: "GET", Target: "/api/sync/files/reports/x.html", Token: token})

	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); strings.Contains(got, "text/html") {
		t.Errorf("Content-Type = %q, must not be renderable as HTML", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if !bytes.Equal(w.Body.Bytes(), page) {
		t.Error("the body must still be the bytes that were stored")
	}
}

// The same endpoint with no Authorization header answers 401, which is what
// keeps a lure link from reaching a stored page through a browser at all.
func TestSyncFilesRefuseAnUnauthenticatedRead(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "secret")
	h := s.Handler()

	w := doReq(t, h, rawCall{Method: "GET", Target: "/api/sync/files/reports/x.html"})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", w.Code)
	}
}
