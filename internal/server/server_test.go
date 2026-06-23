package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func loginAs(t *testing.T, h http.Handler, password, machine string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password, "machine": machine})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login(%q): status %d, body %s", machine, w.Code, w.Body.String())
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return res.Token
}

func countnamed(s *Server, name string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, v := range s.tokens {
		if v.Name == name {
			n++
		}
	}
	return n
}

func TestLoginRotatesTokenPerMachine(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	first := loginAs(t, h, "secret", "lucy")
	second := loginAs(t, h, "secret", "lucy")

	if first == second {
		t.Fatal("re-login should issue a fresh token")
	}
	s.mu.RLock()
	_, oldAlive := s.tokens[first]
	_, newAlive := s.tokens[second]
	s.mu.RUnlock()
	if oldAlive {
		t.Fatal("previous token for machine should be revoked")
	}
	if !newAlive {
		t.Fatal("new token should be valid")
	}
	if got := countnamed(s, "lucy"); got != 1 {
		t.Fatalf("expected exactly 1 lucy token, got %d", got)
	}

	loginAs(t, h, "secret", "ruche")
	if got := countnamed(s, "lucy"); got != 1 {
		t.Fatalf("logging in another machine must not touch lucy, got %d", got)
	}
	if got := countnamed(s, "ruche"); got != 1 {
		t.Fatalf("expected 1 ruche token, got %d", got)
	}
}

func TestLoginBlankMachineFallsBackToSession(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	loginAs(t, h, "secret", "")
	if got := countnamed(s, "session"); got != 1 {
		t.Fatalf("blank machine should be named session, got %d", got)
	}
}
