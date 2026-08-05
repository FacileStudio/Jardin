package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRecovererTurnsPanicInto500(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})

	w := httptest.NewRecorder()
	recoverer(mux).ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panicking handler: status %d, want %d", w.Code, http.StatusInternalServerError)
	}

	if _, bare := New(t.TempDir(), "secret").Handler().(*http.ServeMux); bare {
		t.Fatal("Handler() returns a bare mux: panic recovery is not wired in")
	}
}
