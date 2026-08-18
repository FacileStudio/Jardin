package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
)

func indexStatusGet(t *testing.T, s *Server, query string) (int, MemoryIndexStatusResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/memory/index/status"+query, nil))
	var res MemoryIndexStatusResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, res
}

func attachFake(_ *testing.T, s *Server, hashes map[string]string) {
	s.Semantic = &Semantic{Backend: &fakeBackend{}, Store: &fakeStore{hashes: hashes}}
}

func setupIdle(t *testing.T, s *Server) { attachFake(t, s, nil) }

func setupThreeHashes(t *testing.T, s *Server) {
	attachFake(t, s, map[string]string{"a#1": "x", "b#2": "y", "c#3": "z"})
}

// setupQueued attaches a worker holding one un-drained page, which is what an
// index in flight looks like from the outside.
func setupQueued(t *testing.T, s *Server) {
	t.Helper()
	attachFake(t, s, nil)
	w := NewEmbedWorker(s)
	if w == nil {
		t.Fatal("worker must exist when Semantic is set")
	}
	w.Enqueue(s.DataDir, filepath.Join(s.DataDir, "memory", "tools", "sync.md"))
}

func checkDisabled(t *testing.T, res MemoryIndexStatusResponse) {
	t.Helper()
	if (res != MemoryIndexStatusResponse{}) {
		t.Errorf("without a backend every field must be zero, got %+v", res)
	}
}

func checkThreeHashes(t *testing.T, res MemoryIndexStatusResponse) {
	t.Helper()
	if !res.Enabled {
		t.Fatal("enabled must be true with a backend")
	}
	if res.IndexedChunks != 3 {
		t.Errorf("indexed_chunks = %d, want 3", res.IndexedChunks)
	}
	if res.TotalChunks == 0 {
		t.Error("total_chunks must count the wiki")
	}
	if res.Model.Name != "fake" {
		t.Errorf("model = %+v, want the store's", res.Model)
	}
}

func checkIdle(t *testing.T, res MemoryIndexStatusResponse) {
	t.Helper()
	if res.Indexing || res.PendingPaths != 0 {
		t.Errorf("idle worker reported indexing=%v pending=%d", res.Indexing, res.PendingPaths)
	}
	if res.ChunksPerSecond != 0 || res.ETASeconds != 0 {
		t.Errorf("idle pace = %v/%ds, want zero", res.ChunksPerSecond, res.ETASeconds)
	}
	if res.StartedAt != "" || res.LastError != "" {
		t.Errorf("idle worker reported started_at=%q last_error=%q", res.StartedAt, res.LastError)
	}
}

func checkQueued(t *testing.T, res MemoryIndexStatusResponse) {
	t.Helper()
	if !res.Indexing {
		t.Error("indexing must be true while the queue holds work")
	}
	if res.PendingPaths != 1 {
		t.Errorf("pending_paths = %d, want 1", res.PendingPaths)
	}
}

// TestMemoryIndexStatus is the endpoint's contract: it answers 200 in every
// configuration the dashboard can meet, including no embedding at all, and only
// refuses a space the caller has no claim to.
func TestMemoryIndexStatus(t *testing.T) {
	cases := []struct {
		name  string
		query string
		setup func(t *testing.T, s *Server)
		code  int
		check func(t *testing.T, res MemoryIndexStatusResponse)
	}{
		{name: "no embedding backend", code: http.StatusOK, check: checkDisabled},
		{name: "indexed chunks come from the store", setup: setupThreeHashes, code: http.StatusOK, check: checkThreeHashes},
		{name: "idle worker has no pace", setup: setupIdle, code: http.StatusOK, check: checkIdle},
		{name: "queued work is indexing", setup: setupQueued, code: http.StatusOK, check: checkQueued},
		{name: "unknown space", query: "?space_id=nope", setup: setupIdle, code: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := wikiServer(t)
			if tc.setup != nil {
				tc.setup(t, s)
			}
			code, res := indexStatusGet(t, s, tc.query)
			if code != tc.code {
				t.Fatalf("status %d, want %d", code, tc.code)
			}
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

// TestIndexPaceUsesThePassInFlight checks the arithmetic the progress bar
// depends on: a rate measured over the current pass, and an ETA for what is
// left rather than for what is done.
func TestIndexPaceUsesThePassInFlight(t *testing.T) {
	start := mustTime(t, "2026-08-19T09:12:03Z")
	now := mustTime(t, "2026-08-19T09:22:03Z")
	status := EmbedStatus{Indexing: true, Processed: 300, StartedAt: start}

	rate, eta := indexPace(status, 600, now)
	if rate != 0.5 || eta != 1200 {
		t.Errorf("pace = %v/%ds, want 0.5/1200s", rate, eta)
	}
	if rate, eta := indexPace(status, 0, now); rate != 0.5 || eta != 0 {
		t.Errorf("a finished index reported %v/%ds, want 0.5/0s", rate, eta)
	}
	if rate, eta := indexPace(EmbedStatus{}, 600, now); rate != 0 || eta != 0 {
		t.Errorf("idle pace = %v/%ds, want zero", rate, eta)
	}
}

// TestStoreKindNamesTheImplementation guards the one field the handler cannot
// get from an interface method.
func TestStoreKindNamesTheImplementation(t *testing.T) {
	flat, err := memory.OpenFlatStore(t.TempDir(), memory.ModelID{Name: "fake", Dims: 3})
	if err != nil {
		t.Fatalf("open flat store: %v", err)
	}
	if kind := storeKind(flat); kind != "flat" {
		t.Errorf("storeKind(flat) = %q, want flat", kind)
	}
	if kind := storeKind(&fakeStore{}); kind != "" {
		t.Errorf("storeKind(fake) = %q, want empty", kind)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return at
}
