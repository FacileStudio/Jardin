package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/env"
)

// commonAllowed decides who reaches the instance owner's unscoped tree, so each
// arm is pinned rather than left to the one arm a happy-path test happens to
// walk. A machine token predating multi-user carries no email and must keep
// working; one bound to a demoted user must not.
func TestCommonAllowedByScopeAndUser(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	users := srv.loadUsers()
	users["boss@example.test"] = User{Email: "boss@example.test", Admin: true}
	users["staff@example.test"] = User{Email: "staff@example.test", Admin: false}
	if err := srv.saveUsers(users); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		id   Identity
		want bool
	}{
		"admin scope":               {Identity{Scope: scopeAdmin}, true},
		"admin scope with email":    {Identity{Scope: scopeAdmin, Email: "staff@example.test"}, true},
		"sync token, no email":      {Identity{Scope: scopeSync}, true},
		"sync token, admin user":    {Identity{Scope: scopeSync, Email: "boss@example.test"}, true},
		"sync token, ordinary user": {Identity{Scope: scopeSync, Email: "staff@example.test"}, false},
		"sync token, unknown user":  {Identity{Scope: scopeSync, Email: "ghost@example.test"}, false},
		"user scope":                {Identity{Scope: scopeUser, Email: "staff@example.test"}, false},
		"no scope at all":           {Identity{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := srv.commonAllowed(tc.id); got != tc.want {
				t.Errorf("commonAllowed(%+v) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// spacesMemberAdd is the grant path: it decides who may hand out access and
// which roles they may hand out. Every refusal is pinned with the status it
// answers today, because a refactor that turned one of these into a 201 would
// be a privilege escalation that no type check would catch.
func TestSpacesMemberAddRefusals(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	srv.Log = slog.Default()
	h := srv.Handler()

	owner := sessionFor(t, srv, "owner@example.test", false)
	outsider := sessionFor(t, srv, "outsider@example.test", false)
	sessionFor(t, srv, "invitee@example.test", false)
	spaceID := createSpace(t, h, owner, "Team")

	for name, tc := range map[string]struct {
		token string
		body  string
		want  int
	}{
		"owner adds a member":        {owner, `{"email":"invitee@example.test"}`, http.StatusCreated},
		"owner grants owner":         {owner, `{"email":"invitee@example.test","role":"owner"}`, http.StatusCreated},
		"non-member is not told":     {outsider, `{"email":"invitee@example.test"}`, http.StatusNotFound},
		"unknown user is refused":    {owner, `{"email":"ghost@example.test"}`, http.StatusBadRequest},
		"missing email is refused":   {owner, `{"role":"member"}`, http.StatusBadRequest},
		"invalid role is refused":    {owner, `{"email":"invitee@example.test","role":"wizard"}`, http.StatusBadRequest},
		"malformed body is refused":  {owner, `{`, http.StatusBadRequest},
		"unauthenticated is refused": {"", `{"email":"invitee@example.test"}`, http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: tc.token, Body: tc.body})
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// A member who is not an owner or admin may not grant access at all, and an
// admin may not create another owner. Both are separate arms of the role gate.
func TestSpacesMemberAddRoleGate(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	srv.Log = slog.Default()
	h := srv.Handler()

	owner := sessionFor(t, srv, "owner@example.test", false)
	admin := sessionFor(t, srv, "admin@example.test", false)
	plain := sessionFor(t, srv, "plain@example.test", false)
	sessionFor(t, srv, "target@example.test", false)
	spaceID := createSpace(t, h, owner, "Team")

	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: owner, Body: `{"email":"admin@example.test","role":"admin"}`}); rec.Code != http.StatusCreated {
		t.Fatalf("seeding an admin answered %d", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: owner, Body: `{"email":"plain@example.test","role":"member"}`}); rec.Code != http.StatusCreated {
		t.Fatalf("seeding a member answered %d", rec.Code)
	}

	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: plain, Body: `{"email":"target@example.test"}`}); rec.Code != http.StatusForbidden {
		t.Errorf("a plain member granting access answered %d, want 403", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: admin, Body: `{"email":"target@example.test","role":"owner"}`}); rec.Code != http.StatusForbidden {
		t.Errorf("an admin granting owner answered %d, want 403", rec.Code)
	}
	if rec := spReq(t, h, apiCall{Method: "POST", Path: "/api/spaces/" + spaceID + "/members", Token: admin, Body: `{"email":"target@example.test","role":"member"}`}); rec.Code != http.StatusCreated {
		t.Errorf("an admin granting member answered %d, want 201", rec.Code)
	}
}

func oidcTestServer(t *testing.T) (*Server, *httptest.Server, *http.Client) {
	t.Helper()
	const clientID = "mycelium"
	idp := stubIdP(t, clientID, "yann@facile.studio")
	srv := New(t.TempDir(), "secret")
	srv.Log = slog.Default()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	srv.OIDC = &env.OIDC{
		Issuer:       idp.URL,
		ClientID:     clientID,
		ClientSecret: "secret",
		RedirectURL:  ts.URL + "/api/auth/oidc/callback",
		SuccessURL:   ts.URL + "/auth/callback",
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return srv, ts, client
}

// The browser half of the callback, which the CLI end-to-end tests never walk:
// it must hand the session back in the URL fragment, never the query string,
// so the token does not travel in a referer or a proxy log.
func TestOIDCCallbackBrowserFlowReturnsTheTokenInTheFragment(t *testing.T) {
	_, ts, client := oidcTestServer(t)

	start, err := client.Get(ts.URL + "/api/auth/oidc")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	authorize, err := url.Parse(start.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code=provider-code&state="+authorize.Query().Get("state"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range start.Cookies() {
		req.AddCookie(cookie)
	}
	done, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	done.Body.Close()

	if done.StatusCode != http.StatusFound {
		t.Fatalf("callback answered %d, want a redirect", done.StatusCode)
	}
	dest := done.Header.Get("Location")
	if !strings.Contains(dest, "#token=") {
		t.Fatalf("redirect %q carries no token fragment", dest)
	}
	if strings.Contains(dest, "?token=") || strings.Contains(dest, "&token=") {
		t.Fatalf("the session token travelled in the query string: %q", dest)
	}
}

// Both state failures answer the same way, and neither may reach the provider.
func TestOIDCCallbackRefusesABadState(t *testing.T) {
	_, ts, client := oidcTestServer(t)

	start, err := client.Get(ts.URL + "/api/auth/oidc")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()

	noCookie, err := client.Get(ts.URL + "/api/auth/oidc/callback?code=provider-code&state=whatever")
	if err != nil {
		t.Fatal(err)
	}
	noCookie.Body.Close()
	if noCookie.StatusCode != http.StatusBadRequest {
		t.Errorf("a callback with no state cookie answered %d, want 400", noCookie.StatusCode)
	}

	req, err := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code=provider-code&state=not-the-one", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range start.Cookies() {
		req.AddCookie(cookie)
	}
	mismatch, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	mismatch.Body.Close()
	if mismatch.StatusCode != http.StatusBadRequest {
		t.Errorf("a mismatched state answered %d, want 400", mismatch.StatusCode)
	}
}
