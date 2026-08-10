package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/env"
)

// stubIdP is the smallest thing go-oidc will accept as a provider: discovery,
// a JWKS, and a token endpoint that returns a signed id_token. It exists so the
// CLI flow can be driven end to end without Authentik, and it is hand-rolled
// rather than pulled from a library because a JWT is three base64 segments and
// a signature.
func stubIdP(t *testing.T, clientID, email string) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                server.URL,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"jwks_uri":                              server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": "stub",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   "AQAB",
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()
		idToken := signJWT(t, key, map[string]any{
			"iss":   server.URL,
			"aud":   clientID,
			"sub":   "stub-subject",
			"iat":   now.Unix(),
			"exp":   now.Add(time.Hour).Unix(),
			"email": email,
			"name":  "Yann",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "stub-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	})
	return server
}

func signJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signing := encode(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "stub"}) + "." + encode(claims)
	digest := crypto.SHA256.New()
	digest.Write([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// TestTheCLIFlowEndToEnd walks the path a `mycelium login` takes: start with the
// CLI parameters, come back from the provider, land on the loopback redirect,
// and trade the code for a token that authenticates — once.
func TestTheCLIFlowEndToEnd(t *testing.T) {
	const clientID = "mycelium"
	const email = "yann@facile.studio"
	idp := stubIdP(t, clientID, email)

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

	start, err := client.Get(ts.URL + "/api/auth/oidc?flow=cli&port=51234&cli_state=deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	if start.StatusCode != http.StatusFound {
		t.Fatalf("start answered %d, want a redirect to the provider", start.StatusCode)
	}
	authorize, err := url.Parse(start.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := authorize.Query().Get("state")
	if state == "" {
		t.Fatal("the provider was sent no state")
	}

	callback, err := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code=provider-code&state="+state, nil)
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
	if done.StatusCode != http.StatusFound {
		t.Fatalf("the callback answered %d, want the loopback redirect", done.StatusCode)
	}

	loopback, err := url.Parse(done.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loopback.Scheme != "http" || loopback.Host != "127.0.0.1:51234" {
		t.Fatalf("the browser was sent to %s, want the loopback listener", loopback)
	}
	if loopback.Query().Get("state") != "deadbeef" {
		t.Fatalf("the nonce came back as %q", loopback.Query().Get("state"))
	}
	code := loopback.Query().Get("code")
	if code == "" {
		t.Fatal("the loopback redirect carries no code")
	}

	status, body := doJSON(t, ts, "POST", "/api/auth/oidc/exchange", "", map[string]string{"code": code})
	if status != http.StatusOK || body["token"] == "" {
		t.Fatalf("exchange: %d %v", status, body)
	}
	if status, _ := doJSON(t, ts, "GET", "/api/auth/me", body["token"], nil); status != http.StatusOK {
		t.Fatalf("the exchanged token does not authenticate: %d", status)
	}
	if replay, _ := doJSON(t, ts, "POST", "/api/auth/oidc/exchange", "", map[string]string{"code": code}); replay != http.StatusUnauthorized {
		t.Fatalf("replaying the code answered %d, want 401", replay)
	}
}

// The same walk without a nonce, which is what a mycelium binary installed before
// this flow existed does. It must still reach a loopback redirect with a code.
func TestTheCLIFlowEndToEndWithoutANonce(t *testing.T) {
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
	start, err := client.Get(ts.URL + "/api/auth/oidc?flow=cli&port=51234")
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

	loopback, err := url.Parse(done.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loopback.Host != "127.0.0.1:51234" || loopback.Query().Get("code") == "" {
		t.Fatalf("an old binary's login ended at %s", loopback)
	}
	if _, present := loopback.Query()["state"]; present {
		t.Fatal("no nonce was sent, so none may be echoed")
	}
}

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
