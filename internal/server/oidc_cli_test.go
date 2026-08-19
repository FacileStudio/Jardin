package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// codeFor drives the half of the callback that runs after the IdP has done its
// part, and returns the one-time code the CLI's listener would have received.
func codeFor(t *testing.T, srv *Server, port, nonce string) (string, *url.URL) {
	t.Helper()
	recorder := httptest.NewRecorder()
	srv.issueLoginCode(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback", nil),
		loginCodeGrant{Email: "yann@facile.studio", Scope: scopeUser, Port: port, Nonce: nonce})
	if recorder.Code != http.StatusFound {
		t.Fatalf("callback answered %d, want a redirect", recorder.Code)
	}
	target, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("the redirect is not a URL: %v", err)
	}
	return target.Query().Get("code"), target
}

// A CLI's nonce must come back on the loopback redirect, or its listener has no
// way to tell its own callback from one a local process raced in first.
func TestLoopbackRedirectEchoesTheCLINonce(t *testing.T) {
	srv := New(t.TempDir(), "secret")

	code, target := codeFor(t, srv, "51234", "deadbeef")
	if target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() != "51234" {
		t.Fatalf("redirect went to %s, want loopback on 51234", target)
	}
	if got := target.Query().Get("state"); got != "deadbeef" {
		t.Fatalf("state is %q, want the nonce back", got)
	}
	if code == "" {
		t.Fatal("the redirect carries no code")
	}
}

// A mycelium binary installed before this flow existed sends no nonce. If it
// stops working, deploying the server locks out every machine already set up.
func TestLoopbackRedirectOmitsStateWhenNoNonceWasSent(t *testing.T) {
	srv := New(t.TempDir(), "secret")

	code, target := codeFor(t, srv, "51234", "")
	if _, present := target.Query()["state"]; present {
		t.Fatal("no nonce was sent, so none may be echoed")
	}
	if code == "" {
		t.Fatal("the redirect carries no code")
	}
}

func TestCLIStateRejectsAnythingThatIsNotANonce(t *testing.T) {
	cases := map[string]string{
		"plain":       "deadbeef",
		"mixed":       "aZ09-_",
		"empty":       "",
		"ampersand":   "abc&code=stolen",
		"newline":     "abc\r\nX-Injected: 1",
		"space":       "abc def",
		"percent":     "abc%26",
		"overlong":    strings.Repeat("a", maxCLIState+1),
		"at the edge": strings.Repeat("a", maxCLIState),
	}
	valid := map[string]bool{"plain": true, "mixed": true, "at the edge": true}

	for name, input := range cases {
		got := cliState(input)
		if valid[name] && got != input {
			t.Errorf("%s: %q was rejected", name, input)
		}
		if !valid[name] && got != "" {
			t.Errorf("%s: %q was accepted as %q", name, input, got)
		}
	}
}

func TestLoopbackPortRejectsAnythingButAPort(t *testing.T) {
	cases := map[string]string{
		"unprivileged": "1024",
		"high":         "65535",
		"typical":      "51234",
		"empty":        "",
		"privileged":   "80",
		"zero":         "0",
		"too high":     "65536",
		"negative":     "-1",
		"not a number": "abc",
		"a host":       "evil.example.com:80",
		"trailing":     "8080x",
	}
	valid := map[string]bool{"unprivileged": true, "high": true, "typical": true}

	for name, input := range cases {
		got := loopbackPort(input)
		if valid[name] && got != input {
			t.Errorf("%s: %q was rejected", name, input)
		}
		if !valid[name] && got != "" {
			t.Errorf("%s: %q was accepted as %q", name, input, got)
		}
	}
}

// A port that is not a port is refused before the browser leaves for the IdP,
// and is never allowed to become the host of the redirect back.
// A CLI flow without a usable port is refused up front: with one, the request
// gets past validation and only then finds that this test server has no
// identity provider configured.
func TestStartRefusesACLIFlowWithoutAUsablePort(t *testing.T) {
	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for name, query := range map[string]string{
		"a host":  "?flow=cli&port=evil.example.com:80",
		"missing": "?flow=cli",
		"zero":    "?flow=cli&port=0",
	} {
		resp, err := client.Get(ts.URL + "/api/auth/oidc" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: answered %d, want 400", name, resp.StatusCode)
		}
	}

	resp, err := client.Get(ts.URL + "/api/auth/oidc?flow=cli&port=51234")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("a valid port answered %d, want 503 from the missing provider", resp.StatusCode)
	}
}

func TestTheFlowCookieCarriesTheCLIParametersToTheCallback(t *testing.T) {
	encoded, err := oidcFlow{State: "state", CLI: true, Port: "51234", CLIState: "nonce"}.encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := decodeOIDCFlow(encoded)
	if !ok {
		t.Fatal("the cookie did not decode")
	}
	if decoded.State != "state" || !decoded.CLI || decoded.Port != "51234" || decoded.CLIState != "nonce" {
		t.Fatalf("the cookie lost something: %+v", decoded)
	}
	if _, ok := decodeOIDCFlow("not base64url $$"); ok {
		t.Fatal("garbage decoded as a flow")
	}
	if _, ok := decodeOIDCFlow(""); ok {
		t.Fatal("an empty cookie decoded as a flow")
	}
}

// The exchanged code is minted into a working credential and consumed: a
// second exchange for the same code is refused.
func TestALoginCodeIsExchangedOnceAndOnlyOnce(t *testing.T) {
	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	code, _ := codeFor(t, srv, "51234", "deadbeef")

	status, body := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}})
	if status != http.StatusOK {
		t.Fatalf("exchange: %d %v", status, body)
	}
	if body["token"] == "" {
		t.Fatal("the exchange returned no token")
	}

	if status, _ := doJSON(t, ts, jsonCall{Method: "GET", Path: "/api/auth/me", Token: body["token"], Payload: nil}); status != http.StatusOK {
		t.Fatalf("the exchanged token does not authenticate: %d", status)
	}

	replay, _ := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: map[string]string{"code": code}})
	if replay != http.StatusUnauthorized {
		t.Fatalf("replay answered %d, want 401", replay)
	}
}

func TestAnExchangeWithoutAValidCodeIsRefused(t *testing.T) {
	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	for name, payload := range map[string]map[string]string{
		"missing": {},
		"empty":   {"code": ""},
		"unknown": {"code": "0123456789abcdef"},
	} {
		status, _ := doJSON(t, ts, jsonCall{Method: "POST", Path: "/api/auth/oidc/exchange", Token: "", Payload: payload})
		if status == http.StatusOK {
			t.Errorf("%s: the exchange succeeded", name)
		}
	}
}

func TestAnExpiredLoginCodeIsRefused(t *testing.T) {
	store := newLoginCodeStore()
	now := time.Now().UTC()
	store.create(hashToken("code"), "yann@facile.studio", scopeUser, now)

	if _, ok := store.consume(hashToken("code"), now.Add(loginCodeTTL+time.Second)); ok {
		t.Fatal("an expired code was accepted")
	}
}

// Discovery is how the CLI decides between the browser and the device flow
// before it opens anything.
func TestDiscoveryAdvertisesBothFlows(t *testing.T) {
	srv := New(t.TempDir(), "secret")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/auth/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var config map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	if _, ok := config["oidc_enabled"]; !ok {
		t.Fatal("discovery does not say whether OIDC is enabled")
	}
	if !config["device_enabled"] {
		t.Fatal("discovery does not advertise the device flow")
	}
}
