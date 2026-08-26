package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestAPasswordLoginIsASessionAndNotAnAPIToken pins both halves of the fix that
// stopped the tokens page listing logins. The list has to hide a browser login,
// and it can only do that if the browser login carries the expiry the filter
// reads, which is what mintLogin exists for.
func TestAPasswordLoginIsASessionAndNotAnAPIToken(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	loginAs(t, h, "secret", "lucy")
	admin := loginAs(t, h, "secret", "")

	w := doReq(t, h, rawCall{Method: "GET", Target: "/api/tokens", Token: admin, Body: nil})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tokens: got %d", w.Code)
	}
	var list []TokenInfo
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}

	names := make(map[string]bool, len(list))
	for _, entry := range list {
		names[entry.Name] = true
	}
	if names[passwordSessionName] {
		t.Error("a browser login is listed as an API token, with a revoke button beside it")
	}
	if !names["lucy"] {
		t.Error("a machine token is missing from the list it belongs in")
	}
}

// TestABrowserLoginExpiresAndAMachineTokenDoesNot states the invariant the
// tokens filter reads, at the mint site where it is decided.
func TestABrowserLoginExpiresAndAMachineTokenDoesNot(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	loginAs(t, h, "secret", "lucy")
	loginAs(t, h, "secret", "")

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, info := range s.tokens {
		expires := info.ExpiresAt != ""
		if info.Name == passwordSessionName && !expires {
			t.Error("a browser session never expires, so nothing can tell it from an API token")
		}
		if info.Name == "lucy" && expires {
			t.Error("a machine token expires, which would strand the machine holding it")
		}
	}
}
