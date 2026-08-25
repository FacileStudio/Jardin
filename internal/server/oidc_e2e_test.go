package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
