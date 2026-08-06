package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/sessions"
)

var e0 = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

func mkBlock(id, machine string, end time.Time) sessions.Block {
	return sessions.Block{ID: id, Project: "Mycelium", Machine: machine, Agent: "claude",
		StartedAt: end.Add(-10 * time.Minute), EndedAt: end}
}

// A replaced pool client keeps firing callbacks from its own goroutines. If the
// emitter listened to them it would record the retired client's disconnect as
// its own, tear down a healthy connection on the next tick, and churn the pool.
func TestRetiredClientCallbacksAreIgnored(t *testing.T) {
	e := &Emitter{srv: &Server{Log: slog.Default()}}

	retired := e.disconnect()
	current := e.disconnect()

	e.setConnected(current, true)
	e.setConnected(retired, false)
	e.clientError(retired, "websocket: close 1006")

	if !e.connected {
		t.Fatal("a retired client's disconnect marked the live client as down")
	}
	if e.lastError != "" {
		t.Fatalf("a retired client's error surfaced as %q", e.lastError)
	}
}

func TestPendingBlocksFiltersLedgerAndWatermark(t *testing.T) {
	nook := &NookSettings{
		Enabled:   true,
		UserEmail: "sara@example.com",
		EmitSince: e0.Format(time.RFC3339),
	}
	blocks := []sessions.Block{
		mkBlock("old", "lucy", e0.Add(-time.Hour)),
		mkBlock("done", "lucy", e0.Add(time.Hour)),
		mkBlock("new", "lucy", e0.Add(2*time.Hour)),
	}
	ledger := map[string]string{"done": e0.Format(time.RFC3339)}

	pending := pendingBlocks(blocks, ledger, nook)
	if len(pending) != 1 || pending[0].ID != "new" {
		t.Fatalf("expected only 'new' pending, got %+v", pending)
	}
}

func TestPendingBlocksSkipsSubMinuteBlocks(t *testing.T) {
	nook := &NookSettings{Enabled: true, UserEmail: "yann@facile.studio"}
	short := mkBlock("short", "lucy", e0)
	short.StartedAt = short.EndedAt.Add(-30 * time.Second)
	long := mkBlock("long", "lucy", e0)

	pending := pendingBlocks([]sessions.Block{short, long}, map[string]string{}, nook)
	if len(pending) != 1 || pending[0].ID != "long" {
		t.Fatalf("sub-minute block must be excluded, got %+v", pending)
	}
}

func TestPendingBlocksSkipsUnattributable(t *testing.T) {
	nook := &NookSettings{Enabled: true, MachineEmails: map[string]string{"lucy": "sara@example.com"}}
	blocks := []sessions.Block{
		mkBlock("a", "lucy", e0),
		mkBlock("b", "ruche", e0),
	}
	pending := pendingBlocks(blocks, map[string]string{}, nook)
	if len(pending) != 1 || pending[0].Machine != "lucy" {
		t.Fatalf("machine without email must be skipped, got %+v", pending)
	}
}

func TestEnvelopeShape(t *testing.T) {
	b := mkBlock("abc123", "lucy", e0)
	b.Branch = "main"
	b.TokensIn = 10
	b.CacheWrite = 90
	b.TokensOut = 500
	evt := envelopeFor(&b, "sara@example.com")

	if evt.IdempotencyKey != "mycelium_agent_session_created_abc123" {
		t.Fatalf("bad idempotency key: %s", evt.IdempotencyKey)
	}
	if evt.FacileID != "fac_abc123" || evt.Payload.FacileID != "fac_abc123" {
		t.Fatal("facile id mismatch")
	}
	if evt.Payload.TokensIn != 100 {
		t.Fatalf("tokens_in must include cache writes: %d", evt.Payload.TokensIn)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"object":"agent_session"`, `"app":"Mycelium"`, `"user_email":"sara@example.com"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("envelope json missing %s: %s", want, data)
		}
	}
}

func TestSettingsAPIRoundTrip(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	NewEmitter(srv)
	h := srv.Handler()
	token := loginAs(t, h, "pw", "")

	body := `{"nook":{"enabled":true,"instance":"https://nook.example.com","secret":"s3cret","user_email":"sara@example.com"}}`
	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body.String())
	}

	var resp settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Nook.EmitSince == "" {
		t.Fatal("enabling must set the emit watermark")
	}
	if !resp.Nook.Enabled || resp.Nook.Secret != "s3cret" {
		t.Fatalf("settings not persisted: %+v", resp.Nook)
	}

	req = httptest.NewRequest("GET", "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings: %d", rec.Code)
	}
}

func TestSettingsRequiresAdmin(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	syncToken := loginAs(t, h, "pw", "lucy")

	req := httptest.NewRequest("GET", "/api/settings", nil)
	req.Header.Set("Authorization", "Bearer "+syncToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sync token must not read settings: %d", rec.Code)
	}
}

func TestSettingsValidation(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	token := loginAs(t, h, "pw", "")

	for _, body := range []string{
		`{"nook":{"enabled":true}}`,
		`{"nook":{"enabled":true,"instance":"not a url","secret":"x"}}`,
		`{"nook":{"enabled":true,"instance":"https://x.com","secret":"x","emit_since":"yesterday"}}`,
	} {
		req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", body, rec.Code)
		}
	}
}

func TestSettingsFileNotSynced(t *testing.T) {
	dir := t.TempDir()
	srv := New(dir, "pw")
	if err := srv.saveSettings(Settings{Nook: NookSettings{Secret: "hidden"}}); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	token := loginAs(t, h, "pw", "lucy")

	req := httptest.NewRequest("GET", "/api/sync/tree", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "settings") {
		t.Fatalf("settings must not appear in sync tree: %s", rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/sync/files/.settings.json", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("settings file must not be downloadable via sync")
	}
}

func TestSessionsStatsEndpoint(t *testing.T) {
	dir := t.TempDir()
	srv := New(dir, "pw")
	h := srv.Handler()
	token := loginAs(t, h, "pw", "lucy")

	req := httptest.NewRequest("GET", "/api/sessions/stats?since=7d&by=project", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Fatalf("empty stats must serialize as []: %s", rec.Body.String())
	}
}
