package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// stubIdP is the smallest thing go-oidc will accept as a provider: discovery,
// a JWKS, and a token endpoint that returns a signed id_token. It exists so the
// CLI flow can be driven end to end without a real identity provider, and it is hand-rolled
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

// startOIDC drives the browser as far as the provider and hands back the
// response carrying the state cookie, along with the state the provider was
// sent. Both are needed to replay the hop back.
func startOIDC(t *testing.T, ts *httptest.Server, client *http.Client, query string) (*http.Response, string) {
	t.Helper()
	start, err := client.Get(ts.URL + "/api/auth/oidc" + query)
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
	return start, authorize.Query().Get("state")
}

// completeOIDC replays the provider's redirect into the callback, carrying the
// state cookie the start hop set, and returns where the browser is sent next.
func completeOIDC(t *testing.T, ts *httptest.Server, client *http.Client, start *http.Response, state string) *url.URL {
	t.Helper()
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
	target, err := url.Parse(done.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return target
}

// TestTheCLIFlowEndToEnd walks the path a `mycelium login` takes: start with the
// CLI parameters, come back from the provider, land on the loopback redirect,
// and trade the code for a token that authenticates — once.
func TestTheCLIFlowEndToEnd(t *testing.T) {
	_, ts, client := oidcTestServer(t)

	start, state := startOIDC(t, ts, client, "?flow=cli&port=51234&cli_state=deadbeef")
	if state == "" {
		t.Fatal("the provider was sent no state")
	}

	loopback := completeOIDC(t, ts, client, start, state)
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

	status, body := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}})
	if status != http.StatusOK || body["token"] == "" {
		t.Fatalf("exchange: %d %v", status, body)
	}
	if status, _ := doJSON(t, ts, jsonCall{Method: "GET", Path: "/api/auth/me", Token: body["token"], Payload: nil}); status != http.StatusOK {
		t.Fatalf("the exchanged token does not authenticate: %d", status)
	}
	if replay, _ := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}}); replay != http.StatusUnauthorized {
		t.Fatalf("replaying the code answered %d, want 401", replay)
	}
}

// The same walk without a nonce, which is what a mycelium binary installed before
// this flow existed does. It must still reach a loopback redirect with a code.
func TestTheCLIFlowEndToEndWithoutANonce(t *testing.T) {
	_, ts, client := oidcTestServer(t)

	start, state := startOIDC(t, ts, client, "?flow=cli&port=51234")
	loopback := completeOIDC(t, ts, client, start, state)

	if loopback.Host != "127.0.0.1:51234" || loopback.Query().Get("code") == "" {
		t.Fatalf("an old binary's login ended at %s", loopback)
	}
	if _, present := loopback.Query()["state"]; present {
		t.Fatal("no nonce was sent, so none may be echoed")
	}
}

// callbackResponse replays the provider's redirect into the callback without
// asserting what comes back. completeOIDC insists on a 302 and a Location,
// which is exactly what the two cases below are here to question.
func callbackResponse(t *testing.T, ts *httptest.Server, client *http.Client, start *http.Response, state string) (*http.Response, string) {
	t.Helper()
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
	defer done.Body.Close()
	body, err := io.ReadAll(done.Body)
	if err != nil {
		t.Fatal(err)
	}
	return done, string(body)
}

// A CLI login that fails on the identity is still a browser sitting on a
// top-level navigation, so it is redirected exactly as a browser login is. It
// used to read the flow marker as "the caller is a program" and answer the API
// error envelope, which left a failed `mycelium login` staring at
// {"error":{"code":"unauthenticated"}} rendered as a web page. The CLI hears
// about the failure by timing out on its listener, not by reading JSON here.
func TestACLILoginFailureRedirectsInsteadOfAnsweringJSON(t *testing.T) {
	_, ts, client := oidcTestServerAs(t, "")

	start, state := startOIDC(t, ts, client, "?flow=cli&port=51234&cli_state=deadbeef")
	done, body := callbackResponse(t, ts, client, start, state)

	if done.StatusCode != http.StatusFound {
		t.Fatalf("a refused CLI login answered %d, want 302", done.StatusCode)
	}
	if !strings.HasSuffix(done.Header.Get("Location"), "/login?error=sso") {
		t.Fatalf("a refused CLI login ended at %q", done.Header.Get("Location"))
	}
	if ct := done.Header.Get("Content-Type"); strings.Contains(ct, "json") {
		t.Fatalf("a refused CLI login answered %s", ct)
	}
	if strings.Contains(body, "unauthenticated") {
		t.Fatalf("the error envelope reached the browser: %q", body)
	}
}

// A CLI with no loopback listener is the ordinary case on a machine whose
// browser lives elsewhere, and refusing it left `mycelium login` with no route
// through at all. The callback hands back the same one-time code on a page
// instead of a redirect, and that code exchanges for a working token.
func TestACLIFlowWithNoPortHandsBackAPasteCode(t *testing.T) {
	_, ts, client := oidcTestServer(t)

	start, state := startOIDC(t, ts, client, "?flow=cli")
	done, body := callbackResponse(t, ts, client, start, state)

	if done.StatusCode != http.StatusOK {
		t.Fatalf("the paste hand-off answered %d, want 200", done.StatusCode)
	}
	if ct := done.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("the paste hand-off is %s, want html", ct)
	}
	if cache := done.Header.Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("a page carrying a credential answered Cache-Control %q", cache)
	}
	if strings.Contains(body, "127.0.0.1") {
		t.Fatal("a login with no port still built a loopback redirect")
	}
	if !strings.Contains(body, "60 seconds") {
		t.Fatalf("the page does not say how long the code lasts: %q", body)
	}

	code := pasteCode(t, body)
	status, exchanged := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}})
	if status != http.StatusOK || exchanged["token"] == "" {
		t.Fatalf("exchanging the pasted code: %d %v", status, exchanged)
	}
	if status, _ := doJSON(t, ts, jsonCall{Method: "GET", Path: "/api/auth/me", Token: exchanged["token"], Payload: nil}); status != http.StatusOK {
		t.Fatalf("the pasted code's token does not authenticate: %d", status)
	}
	if replay, _ := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}}); replay != http.StatusUnauthorized {
		t.Fatalf("replaying a pasted code answered %d, want 401", replay)
	}
}

// pasteCode reads the code out of the rendered page the way the person does,
// which is also what catches the template silently dropping the field.
func pasteCode(t *testing.T, body string) string {
	t.Helper()
	found := regexp.MustCompile(`<output id="c">([^<]+)</output>`).FindStringSubmatch(body)
	if found == nil {
		t.Fatalf("the hand-off page shows no code: %q", body)
	}
	return found[1]
}
