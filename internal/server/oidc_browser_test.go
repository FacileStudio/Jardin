package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/env"
)

// A browser login must keep landing on the web app, with the token in the
// fragment: adding the CLI flow may not change what the dashboard does.
func TestABrowserLoginStillEndsOnTheWebApp(t *testing.T) {
	const clientID = "mycelium"
	idp := stubIdP(t, clientID, "yann@facile.studio")

	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	srv.OIDC = &env.OIDC{
		Issuer: idp.URL, ClientID: clientID, ClientSecret: "secret",
		RedirectURL: ts.URL + "/api/auth/oidc/callback", SuccessURL: "https://mycelium.example.com/auth/callback",
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	start, err := client.Get(ts.URL + "/api/auth/oidc")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	authorize, err := url.Parse(start.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}

	callback, err := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code=provider-code&state="+authorize.Query().Get("state"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range start.Cookies() {
		callback.AddCookie(cookie)
	}
	done, err := client.Do(callback)
	if err != nil {
		t.Fatal(err)
	}
	done.Body.Close()

	location := done.Header.Get("Location")
	if !strings.HasPrefix(location, "https://mycelium.example.com/auth/callback#token=") {
		t.Fatalf("a browser login ended at %s", location)
	}
}

// A callback whose state does not match the cookie is a forged one.
func TestTheCallbackRefusesATamperedState(t *testing.T) {
	const clientID = "mycelium"
	idp := stubIdP(t, clientID, "yann@facile.studio")

	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	srv.OIDC = &env.OIDC{
		Issuer: idp.URL, ClientID: clientID, ClientSecret: "secret",
		RedirectURL: ts.URL + "/api/auth/oidc/callback", SuccessURL: ts.URL + "/auth/callback",
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	start, err := client.Get(ts.URL + "/api/auth/oidc?flow=cli&port=51234&cli_state=deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()

	callback, err := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code=provider-code&state=somebody-elses", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range start.Cookies() {
		callback.AddCookie(cookie)
	}
	done, err := client.Do(callback)
	if err != nil {
		t.Fatal(err)
	}
	done.Body.Close()
	if done.StatusCode != http.StatusBadRequest {
		t.Fatalf("a tampered state answered %d, want 400", done.StatusCode)
	}
}
