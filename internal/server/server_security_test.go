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

func doReq(t *testing.T, h http.Handler, method, target, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
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
			w := doReq(t, h, c.method, c.target, syncToken, c.body)
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
			w := doReq(t, h, c.method, c.target, adminToken, c.body)
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

	w := doReq(t, h, "GET", "/api/tokens", adminToken, nil)
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
		w := doReq(t, h, "POST", "/api/auth/login", "", body)
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
	w := doReq(t, h, "POST", "/api/auth/login", "", body)
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
		w := doReq(t, h, "GET", "/api/sync/files/tokens.json", token, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("GET tokens.json: got %d, want 400", w.Code)
		}
	})

	t.Run("traversal does not escape data dir", func(t *testing.T) {
		w := doReq(t, h, "GET", "/api/sync/files/..%2f..%2fetc%2fhosts", token, nil)
		if w.Code == http.StatusOK {
			t.Fatalf("traversal returned 200, must not serve out-of-tree file")
		}
		if strings.Contains(w.Body.String(), "localhost") {
			t.Fatal("traversal leaked /etc/hosts contents")
		}
	})

	t.Run("dotfile rejected", func(t *testing.T) {
		w := doReq(t, h, "GET", "/api/sync/files/.secret", token, nil)
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
			get := doReq(t, h, "GET", "/api/rules/"+name, token, nil)
			if get.Code != http.StatusBadRequest {
				t.Errorf("GET /api/rules/%s: got %d, want 400", name, get.Code)
			}
			put := doReq(t, h, "PUT", "/api/rules/"+name, token, []byte("x"))
			if put.Code != http.StatusBadRequest {
				t.Errorf("PUT /api/rules/%s: got %d, want 400", name, put.Code)
			}
		}
	})

	t.Run("normal name round-trips", func(t *testing.T) {
		want := "# my rule\n"
		put := doReq(t, h, "PUT", "/api/rules/myrule", token, []byte(want))
		if put.Code != http.StatusNoContent {
			t.Fatalf("PUT: got %d, want 204", put.Code)
		}
		get := doReq(t, h, "GET", "/api/rules/myrule", token, nil)
		if get.Code != http.StatusOK {
			t.Fatalf("GET: got %d, want 200", get.Code)
		}
		if get.Body.String() != want {
			t.Fatalf("round-trip mismatch: got %q, want %q", get.Body.String(), want)
		}
	})
}
