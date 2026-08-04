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

func spReq(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func createSpace(t *testing.T, h http.Handler, token, name string) string {
	t.Helper()
	rec := spReq(t, h, "POST", "/api/spaces", token, `{"name":"`+name+`"}`)
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

	rec := spReq(t, h, "GET", "/api/spaces", owner, "")
	if !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Fatalf("creator must be owner: %s", rec.Body.String())
	}

	rec = spReq(t, h, "PUT", "/api/spaces/"+id, owner, `{"name":"Facile Studio"}`)
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

	rec := spReq(t, h, "PUT", "/api/rules/test?space_id="+id, owner, "content")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("member write: %d", rec.Code)
	}

	rec = spReq(t, h, "GET", "/api/rules?space_id="+id, outsider, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member must get 403, got %d", rec.Code)
	}
	rec = spReq(t, h, "GET", "/api/sync/tree?space_id="+id, outsider, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member sync must get 403, got %d", rec.Code)
	}

	rec = spReq(t, h, "POST", "/api/spaces/"+id+"/members", owner, `{"email":"noah@facile.studio","role":"member"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add member: %d %s", rec.Code, rec.Body.String())
	}
	rec = spReq(t, h, "GET", "/api/rules?space_id="+id, outsider, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "test") {
		t.Fatalf("member must read space rules: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCommonTreeNeverExposesSpaces(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	owner := sessionFor(t, srv, "yann@facile.studio", false)
	id := createSpace(t, h, owner, "Secret")
	spReq(t, h, "PUT", "/api/rules/hidden?space_id="+id, owner, "secret content")

	machine := loginAs(t, h, "pw", "lucy")

	rec := spReq(t, h, "GET", "/api/sync/tree", machine, "")
	if strings.Contains(rec.Body.String(), "spaces") {
		t.Fatalf("common tree must not list space files: %s", rec.Body.String())
	}
	rec = spReq(t, h, "GET", "/api/sync/files/spaces/"+id+"/rules/hidden.md", machine, "")
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
	spReq(t, h, "POST", "/api/spaces/"+id+"/members", owner, `{"email":"noah@facile.studio","role":"member"}`)

	if rec := spReq(t, h, "PUT", "/api/spaces/"+id, member, `{"name":"Hijacked"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("member update must be forbidden: %d", rec.Code)
	}
	if rec := spReq(t, h, "DELETE", "/api/spaces/"+id, member, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("member delete must be forbidden: %d", rec.Code)
	}
	if rec := spReq(t, h, "POST", "/api/spaces/"+id+"/leave", owner, ""); rec.Code != http.StatusConflict {
		t.Fatalf("last owner leave must 409: %d", rec.Code)
	}
	if rec := spReq(t, h, "POST", "/api/spaces/"+id+"/leave", member, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("member leave: %d", rec.Code)
	}
}

func TestMachineTokensCannotManageSpaces(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	machine := loginAs(t, h, "pw", "lucy")

	if rec := spReq(t, h, "POST", "/api/spaces", machine, `{"name":"X"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("machine token create space must 403: %d", rec.Code)
	}
	if rec := spReq(t, h, "GET", "/api/spaces", machine, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"spaces":[]`) {
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
	if rec := spReq(t, h, "GET", "/api/sync/tree?space_id="+id, machine, ""); rec.Code != http.StatusOK {
		t.Fatalf("member-bound machine token must sync space: %d", rec.Code)
	}
	if rec := spReq(t, h, "PUT", "/api/sync/files/memory/note.md?space_id="+id, machine, "hello"); rec.Code != http.StatusNoContent {
		t.Fatalf("member-bound machine token must write space files: %d", rec.Code)
	}

	orphan := loginAs(t, h, "pw", "stray")
	if rec := spReq(t, h, "GET", "/api/sync/tree?space_id="+id, orphan, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("userless machine token must stay fenced out: %d", rec.Code)
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

	if rec := spReq(t, h, "GET", "/api/auth/me", token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session must 401: %d", rec.Code)
	}
}

func TestAuthMeAndUsers(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	h := srv.Handler()
	token := sessionFor(t, srv, "yann@facile.studio", true)

	rec := spReq(t, h, "GET", "/api/auth/me", token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "yann@facile.studio") {
		t.Fatalf("auth/me: %d %s", rec.Code, rec.Body.String())
	}

	legacy := loginAs(t, h, "pw", "")
	rec = spReq(t, h, "GET", "/api/auth/me", legacy, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin":true`) {
		t.Fatalf("legacy admin auth/me: %d %s", rec.Code, rec.Body.String())
	}

	rec = spReq(t, h, "GET", "/api/users", token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "yann@facile.studio") {
		t.Fatalf("users list: %d %s", rec.Code, rec.Body.String())
	}

	rec = spReq(t, h, "POST", "/api/auth/logout", token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec = spReq(t, h, "GET", "/api/auth/me", token, ""); rec.Code != http.StatusUnauthorized {
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
