package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sessionFor(t *testing.T, srv *Server, email string, admin bool) string {
	t.Helper()
	users := srv.loadUsers()
	users[email] = User{Email: email, Name: email, Admin: admin, CreatedAt: "2026-01-01T00:00:00Z"}
	if err := srv.saveUsers(users); err != nil {
		t.Fatal(err)
	}
	scope := scopeUser
	if admin {
		scope = scopeAdmin
	}
	token, err := srv.mintSessionToken(email, scope)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// apiCall is one request a test makes. The four fields travel as a struct
// rather than as four bare strings, because positionally a swapped path and
// token still compiles and then fails somewhere unrelated.
type apiCall struct {
	Method string
	Path   string
	Token  string
	Body   string
}

func spReq(t *testing.T, h http.Handler, c apiCall) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if c.Body != "" {
		req = httptest.NewRequest(c.Method, c.Path, strings.NewReader(c.Body))
	} else {
		req = httptest.NewRequest(c.Method, c.Path, nil)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func createSpace(t *testing.T, h http.Handler, token, name string) string {
	t.Helper()
	rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces", Token: token, Body: `{"name":"` + name + `"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create space: %d %s", rec.Code, rec.Body.String())
	}
	var space SpaceResponse
	json.Unmarshal(rec.Body.Bytes(), &space)
	return space.ID
}

func TestSpaceLifecycle(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)

	id := createSpace(t, h, owner, "Facile")

	rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/spaces", Token: owner, Body: ""})
	if !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Fatalf("creator must be owner: %s", rec.Body.String())
	}

	rec = spReq(t, h, apiCall{Method: "PUT", Path: "/api/spaces/" + id, Token: owner, Body: `{"name":"Facile Studio"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d", rec.Code)
	}

	if _, err := os.Stat(filepath.Join(srv.DataDir, "spaces", id, "rules")); err != nil {
		t.Fatal("space tree not scaffolded")
	}
}

func TestSpaceMembershipGuardsContent(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)
	outsider := sessionFor(t, srv, "noah@facile.studio", false)

	id := createSpace(t, h, owner, "Facile")

	rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/rules/test?space_id=" + id, Token: owner, Body: "content"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("member write: %d", rec.Code)
	}

	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/rules?space_id=" + id, Token: outsider, Body: ""})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member must get 403, got %d", rec.Code)
	}
	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/tree?space_id=" + id, Token: outsider, Body: ""})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member sync must get 403, got %d", rec.Code)
	}

	rec = spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + id + "/members", Token: owner, Body: `{"email":"noah@facile.studio","role":"member"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add member: %d %s", rec.Code, rec.Body.String())
	}
	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/rules?space_id=" + id, Token: outsider, Body: ""})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "test") {
		t.Fatalf("member must read space rules: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCommonTreeNeverExposesSpaces(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)
	id := createSpace(t, h, owner, "Secret")
	spReq(t, h, apiCall{Method: "PUT", Path: "/api/rules/hidden?space_id=" + id, Token: owner, Body: "secret content"})

	machine := loginAs(t, h, "pw", "lucy")

	rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/tree", Token: machine, Body: ""})
	if strings.Contains(rec.Body.String(), "spaces") {
		t.Fatalf("common tree must not list space files: %s", rec.Body.String())
	}
	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/files/spaces/" + id + "/rules/hidden.md", Token: machine, Body: ""})
	if rec.Code == http.StatusOK {
		t.Fatal("space file must not be readable through the common tree")
	}
}

func TestSpaceRoleEnforcement(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)
	member := sessionFor(t, srv, "noah@facile.studio", false)

	id := createSpace(t, h, owner, "Facile")
	spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + id + "/members", Token: owner, Body: `{"email":"noah@facile.studio","role":"member"}`})

	if rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/spaces/" + id, Token: member, Body: `{"name":"Hijacked"}`}); rec.Code != http.StatusForbidden {
		t.Fatalf("member update must be forbidden: %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "DELETE", Path: "/api/spaces/" + id, Token: member, Body: ""}); rec.Code != http.StatusForbidden {
		t.Fatalf("member delete must be forbidden: %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + id + "/leave", Token: owner, Body: ""}); rec.Code != http.StatusConflict {
		t.Fatalf("last owner leave must 409: %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + id + "/leave", Token: member, Body: ""}); rec.Code != http.StatusNoContent {
		t.Fatalf("member leave: %d", rec.Code)
	}
}

func TestMachineTokensCannotManageSpaces(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	machine := loginAs(t, h, "pw", "lucy")

	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces", Token: machine, Body: `{"name":"X"}`}); rec.Code != http.StatusForbidden {
		t.Fatalf("machine token create space must 403: %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/spaces", Token: machine, Body: ""}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"spaces":[]`) {
		t.Fatalf("machine token space list must be empty 200: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserBoundMachineTokenSyncsSpace(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)
	id := createSpace(t, h, owner, "Facile")

	machine, err := srv.mintToken("lucy", scopeSync, "yann@facile.studio")
	if err != nil {
		t.Fatal(err)
	}
	if rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/tree?space_id=" + id, Token: machine, Body: ""}); rec.Code != http.StatusOK {
		t.Fatalf("member-bound machine token must sync space: %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/sync/files/memory/note.md?space_id=" + id, Token: machine, Body: "hello"}); rec.Code != http.StatusNoContent {
		t.Fatalf("member-bound machine token must write space files: %d", rec.Code)
	}

	orphan := loginAs(t, h, "pw", "stray")
	if rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/tree?space_id=" + id, Token: orphan, Body: ""}); rec.Code != http.StatusForbidden {
		t.Fatalf("userless machine token must stay fenced out: %d", rec.Code)
	}
}

func TestCommonTreeIsOwnerPrivate(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", true)
	other := sessionFor(t, srv, "noah@facile.studio", false)

	if rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/rules/private", Token: owner, Body: "my wiki"}); rec.Code != http.StatusNoContent {
		t.Fatalf("admin must write common: %d", rec.Code)
	}

	for _, path := range []string{
		"/api/rules", "/api/rules/private", "/api/skills", "/api/memory/index",
		"/api/memory/search?q=x", "/api/sessions/stats", "/api/sessions/recent",
		"/api/status", "/api/sync/tree", "/api/sync/files/rules/private.md",
	} {
		if rec := spReq(t, h, apiCall{Method: "GET", Path: path, Token: other, Body: ""}); rec.Code != http.StatusForbidden {
			t.Fatalf("non-admin user must not read common %s: got %d", path, rec.Code)
		}
	}
	if rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/rules/injected", Token: other, Body: "x"}); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin user must not write common: %d", rec.Code)
	}

	machine := loginAs(t, h, "pw", "lucy")
	if rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/sync/tree", Token: machine, Body: ""}); rec.Code != http.StatusOK {
		t.Fatalf("machine token must keep syncing common: %d", rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	token := sessionFor(t, srv, "yann@facile.studio", true)

	hash := hashToken(token)
	srv.mu.Lock()
	info := srv.tokens[hash]
	info.ExpiresAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	srv.tokens[hash] = info
	srv.mu.Unlock()

	if rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/auth/me", Token: token, Body: ""}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session must 401: %d", rec.Code)
	}
}

func TestAuthMeAndUsers(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	token := sessionFor(t, srv, "yann@facile.studio", true)

	rec := spReq(t, h, apiCall{Method: "GET", Path: "/api/auth/me", Token: token, Body: ""})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "yann@facile.studio") {
		t.Fatalf("auth/me: %d %s", rec.Code, rec.Body.String())
	}

	legacy := loginAs(t, h, "pw", "")
	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/auth/me", Token: legacy, Body: ""})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin":true`) {
		t.Fatalf("legacy admin auth/me: %d %s", rec.Code, rec.Body.String())
	}

	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/users", Token: token, Body: ""})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "yann@facile.studio") {
		t.Fatalf("users list: %d %s", rec.Code, rec.Body.String())
	}

	rec = spReq(t, h, apiCall{Method: "POST", Path: "/api/auth/logout", Token: token, Body: ""})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/auth/me", Token: token, Body: ""}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token must be dead after logout: %d", rec.Code)
	}
}

func TestFirstUserBecomesAdmin(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	first := srv.upsertUser("yann@facile.studio", "Yann")
	second := srv.upsertUser("noah@facile.studio", "Noah")
	if !first.Admin || second.Admin {
		t.Fatalf("first user admin bootstrap broken: first=%v second=%v", first.Admin, second.Admin)
	}
}

func TestUserUpdateAdminPromotion(t *testing.T) {
	dir := t.TempDir()
	srv := New(dir, "pw")
	h := srv.Handler()

	adminToken := sessionFor(t, srv, "yann@facile.studio", true)
	userToken := sessionFor(t, srv, "noah@facile.studio", false)

	rec := spReq(t, h, apiCall{Method: "PUT", Path: "/api/users/noah@facile.studio", Token: userToken, Body: `{"admin":true}`})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin must not promote users: got %d", rec.Code)
	}

	rec = spReq(t, h, apiCall{Method: "PUT", Path: "/api/users/noah@facile.studio", Token: adminToken, Body: `{"admin":true}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("admin promotion failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = spReq(t, h, apiCall{Method: "GET", Path: "/api/auth/me", Token: userToken, Body: ""})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin":true`) {
		t.Fatalf("promoted user must immediately have admin scope: %d %s", rec.Code, rec.Body.String())
	}
}
