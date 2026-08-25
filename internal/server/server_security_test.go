package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// rawCall is one request with a byte body, kept apart from apiCall because a
// nil body and an empty one are different requests here.
type rawCall struct {
	Method string
	Target string
	Token  string
	Body   []byte
}

func doReq(t *testing.T, h http.Handler, c rawCall) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if c.Body != nil {
		r = httptest.NewRequest(c.Method, c.Target, bytes.NewReader(c.Body))
	} else {
		r = httptest.NewRequest(c.Method, c.Target, nil)
	}
	if c.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestTokenHashedAtRest(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, "secret")
	h := s.Handler()

	token := loginAs(t, h, "secret", "lucy")

	s.mu.RLock()
	if _, ok := s.tokens[token]; ok {
		s.mu.RUnlock()
		t.Fatal("raw token must not be a key in s.tokens")
	}
	if _, ok := s.tokens[hashToken(token)]; !ok {
		s.mu.RUnlock()
		t.Fatal("token hash must be a key in s.tokens")
	}
	s.mu.RUnlock()

	data, err := os.ReadFile(s.tokensPath())
	if err != nil {
		t.Fatalf("read tokens.json: %v", err)
	}
	if strings.Contains(string(data), token) {
		t.Fatal("tokens.json must not contain the plaintext token")
	}
	if !strings.Contains(string(data), hashToken(token)) {
		t.Fatal("tokens.json should contain the token hash")
	}
}

func TestScopeEnforcement(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	syncToken := loginAs(t, h, "secret", "lucy")
	adminToken := loginAs(t, h, "secret", "")

	t.Run("sync token forbidden on admin endpoints", func(t *testing.T) {
		cases := []struct {
			method, target string
			body           []byte
		}{
			{"GET", "/api/tokens", nil},
			{"POST", "/api/tokens", []byte(`{"name":"x"}`)},
			{"DELETE", "/api/tokens/lucy", nil},
		}
		for _, c := range cases {
			w := doReq(t, h, rawCall{Method: c.method, Target: c.target, Token: syncToken, Body: c.body})
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s with sync token: got %d, want 403", c.method, c.target, w.Code)
			}
		}
	})

	t.Run("admin token allowed on admin endpoints", func(t *testing.T) {
		cases := []struct {
			method, target string
			body           []byte
			want           int
		}{
			{"GET", "/api/tokens", nil, http.StatusOK},
			{"POST", "/api/tokens", []byte(`{"name":"newmachine"}`), http.StatusOK},
			{"DELETE", "/api/tokens/lucy", nil, http.StatusNoContent},
		}
		for _, c := range cases {
			w := doReq(t, h, rawCall{Method: c.method, Target: c.target, Token: adminToken, Body: c.body})
			if w.Code == http.StatusForbidden {
				t.Errorf("%s %s with admin token: got 403, want %d", c.method, c.target, c.want)
			}
			if w.Code != c.want {
				t.Errorf("%s %s with admin token: got %d, want %d", c.method, c.target, w.Code, c.want)
			}
		}
	})
}

func TestTokensListNeverLeaksSecrets(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	loginAs(t, h, "secret", "lucy")
	adminToken := loginAs(t, h, "secret", "")

	w := doReq(t, h, rawCall{Method: "GET", Target: "/api/tokens", Token: adminToken, Body: nil})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tokens: got %d", w.Code)
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one token entry")
	}
	for _, entry := range list {
		if _, ok := entry["token"]; ok {
			t.Error("token entry must not contain a token field")
		}
		if _, ok := entry["hash"]; ok {
			t.Error("token entry must not contain a hash field")
		}
		for k := range entry {
			switch k {
			case "name", "scope", "created_at", "last_seen":
			default:
				t.Errorf("unexpected field %q in token entry", k)
			}
		}
	}
}

func TestLoginRateLimiting(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	body, _ := json.Marshal(map[string]string{"password": "wrong", "machine": "lucy"})
	saw429 := false
	for i := 0; i < 12; i++ {
		w := doReq(t, h, rawCall{Method: "POST", Target: "/api/auth/login", Token: "", Body: body})
		if w.Code == http.StatusTooManyRequests {
			saw429 = true
			break
		}
	}
	if !saw429 {
		t.Fatal("expected at least one 429 within 12 attempts")
	}
}

func TestConstantTimePasswordCompare(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	body, _ := json.Marshal(map[string]string{"password": "wrong", "machine": "lucy"})
	w := doReq(t, h, rawCall{Method: "POST", Target: "/api/auth/login", Token: "", Body: body})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d, want 401", w.Code)
	}

	token := loginAs(t, h, "secret", "lucy")
	if token == "" {
		t.Fatal("right password should return a token")
	}
}

func TestSyncPathTraversal(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "")

	t.Run("tokens.json rejected", func(t *testing.T) {
		w := doReq(t, h, rawCall{Method: "GET", Target: "/api/sync/files/tokens.json", Token: token, Body: nil})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("GET tokens.json: got %d, want 400", w.Code)
		}
	})

	t.Run("traversal does not escape data dir", func(t *testing.T) {
		w := doReq(t, h, rawCall{Method: "GET", Target: "/api/sync/files/..%2f..%2fetc%2fhosts", Token: token, Body: nil})
		if w.Code == http.StatusOK {
			t.Fatalf("traversal returned 200, must not serve out-of-tree file")
		}
		if strings.Contains(w.Body.String(), "localhost") {
			t.Fatal("traversal leaked /etc/hosts contents")
		}
	})

	t.Run("dotfile rejected", func(t *testing.T) {
		w := doReq(t, h, rawCall{Method: "GET", Target: "/api/sync/files/.secret", Token: token, Body: nil})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("GET dotfile: got %d, want 400", w.Code)
		}
	})
}

func TestSafeNameOnRules(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "")

	t.Run("traversal name rejected", func(t *testing.T) {
		for _, name := range []string{"..%2f..%2fpasswd", "foo%2Fbar", "a..b"} {
			get := doReq(t, h, rawCall{Method: "GET", Target: "/api/rules/" + name, Token: token, Body: nil})
			if get.Code != http.StatusBadRequest {
				t.Errorf("GET /api/rules/%s: got %d, want 400", name, get.Code)
			}
			put := doReq(t, h, rawCall{Method: "PUT", Target: "/api/rules/" + name, Token: token, Body: []byte("x")})
			if put.Code != http.StatusBadRequest {
				t.Errorf("PUT /api/rules/%s: got %d, want 400", name, put.Code)
			}
		}
	})

	t.Run("normal name round-trips", func(t *testing.T) {
		want := "# my rule\n"
		put := doReq(t, h, rawCall{Method: "PUT", Target: "/api/rules/myrule", Token: token, Body: []byte(want)})
		if put.Code != http.StatusNoContent {
			t.Fatalf("PUT: got %d, want 204", put.Code)
		}
		get := doReq(t, h, rawCall{Method: "GET", Target: "/api/rules/myrule", Token: token, Body: nil})
		if get.Code != http.StatusOK {
			t.Fatalf("GET: got %d, want 200", get.Code)
		}
		if get.Body.String() != want {
			t.Fatalf("round-trip mismatch: got %q, want %q", get.Body.String(), want)
		}
	})
}

// TestTokensListOmitsLoginSessions pins the API side of a confusing page: the
// dashboard lists what a person created and manages, and a browser or CLI
// login is neither. The page used to filter these itself and compared the
// whole name against "session", which matches neither "session:<email>" nor
// "cli:<email>", so every login anyone had ever made was rendered as an API
// token with a revoke button next to it.
func TestTokensListOmitsLoginSessions(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()

	adminToken := loginAs(t, h, "secret", "")
	loginAs(t, h, "secret", "lucy")
	if _, err := s.mintSessionToken("yann@facile.studio", "admin"); err != nil {
		t.Fatalf("minting a browser session: %v", err)
	}
	if _, err := s.mintNamedSession("cli:yann@facile.studio", "yann@facile.studio", "user"); err != nil {
		t.Fatalf("minting a CLI session: %v", err)
	}

	w := doReq(t, h, rawCall{Method: "GET", Target: "/api/tokens", Token: adminToken, Body: nil})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tokens: got %d", w.Code)
	}
	var list []TokenInfo
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}

	machine := false
	for _, entry := range list {
		if strings.HasPrefix(entry.Name, "session:") || strings.HasPrefix(entry.Name, "cli:") {
			t.Errorf("a login session reached the tokens list: %q", entry.Name)
		}
		if entry.Name == "lucy" {
			machine = true
		}
	}
	if !machine {
		t.Error("the machine token is what this page is for and it went missing")
	}
}
