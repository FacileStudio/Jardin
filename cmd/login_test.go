package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
)

func loopbackListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener, fmt.Sprintf("http://127.0.0.1:%d/", listener.Addr().(*net.TCPAddr).Port)
}

// TestTheListenerTakesTheCodeFromItsOwnCallback proves the callback carries
// its nonce and that stray browser traffic does not kill the login: a browser
// asks for /favicon.ico unprompted, and the exchange must survive it rather
// than fail on the first stray request.
func TestTheListenerTakesTheCodeFromItsOwnCallback(t *testing.T) {
	listener, base := loopbackListener(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, err := http.Get(base + "favicon.ico"); err == nil {
			resp.Body.Close()
		}
		if resp, err := http.Get(base + "?code=goodcode&state=nonce"); err == nil {
			resp.Body.Close()
		}
	}()

	code, err := awaitLoginCode(listener, "nonce")
	<-done
	if err != nil {
		t.Fatalf("the login failed: %v", err)
	}
	if code != "goodcode" {
		t.Fatalf("code is %q, want the one from the redirect", code)
	}
}

// A callback carrying a code under the wrong nonce is not noise, it is somebody
// else's. Accepting it is the vulnerability this flow exists to avoid.
func TestTheListenerRefusesACallbackWithTheWrongNonce(t *testing.T) {
	listener, base := loopbackListener(t)

	status := make(chan int, 1)
	go func() {
		resp, err := http.Get(base + "?code=stolen&state=notthenonce")
		if err != nil {
			status <- 0
			return
		}
		resp.Body.Close()
		status <- resp.StatusCode
	}()

	code, err := awaitLoginCode(listener, "nonce")
	if !errors.Is(err, errCallbackMismatch) {
		t.Fatalf("error is %v, want a hard abort", err)
	}
	if code != "" {
		t.Fatalf("a code came back from a mismatched callback: %q", code)
	}
	if got := <-status; got != http.StatusBadRequest {
		t.Fatalf("the callback answered %d, want 400", got)
	}
}

// A CLI that sent no nonce could be handed any callback at all, so the client
// half always sends one.
func TestTheListenerRefusesACallbackWithNoNonceAtAll(t *testing.T) {
	listener, base := loopbackListener(t)

	go func() {
		if resp, err := http.Get(base + "?code=stolen"); err == nil {
			resp.Body.Close()
		}
	}()

	if _, err := awaitLoginCode(listener, "nonce"); !errors.Is(err, errCallbackMismatch) {
		t.Fatalf("error is %v, want a hard abort", err)
	}
}

func TestALoginNonceIsRandomAndReflectable(t *testing.T) {
	first, err := loginNonce()
	if err != nil {
		t.Fatal(err)
	}
	second, err := loginNonce()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two nonces came out the same")
	}
	for _, r := range first {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			t.Fatalf("the nonce contains %q, which the server will reject", r)
		}
	}
}

// The environment is the only credential channel a CI job has, and it must not
// be able to write itself into the config file.
func TestTheEnvironmentOverridesTheConfigFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.TokenEnv, "")
	t.Setenv(config.URLEnv, "")
	t.Setenv(config.URLEnvAlt, "")

	stored := &config.MyceliumConfig{
		URL: "https://stored.example.com", Token: "stored-token",
		Machine: "lucy", Space: "space-1", UsageToken: "usage",
		RuleOrder: []string{"one"}, Agents: []string{"claude"},
	}
	if err := config.SaveMyceliumConfig(stored); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerURL() != "https://stored.example.com" || cfg.AuthToken() != "stored-token" {
		t.Fatalf("the config file is not the fallback: %q %q", cfg.ServerURL(), cfg.AuthToken())
	}

	t.Setenv(config.TokenEnv, "env-token")
	t.Setenv(config.URLEnv, "https://env.example.com/")
	if cfg.ServerURL() != "https://env.example.com" {
		t.Fatalf("URL is %q, want the environment's without its trailing slash", cfg.ServerURL())
	}
	if cfg.AuthToken() != "env-token" {
		t.Fatalf("token is %q, want the environment's", cfg.AuthToken())
	}

	t.Setenv(config.URLEnv, "")
	t.Setenv(config.URLEnvAlt, "https://suite.example.com")
	if cfg.ServerURL() != "https://suite.example.com" {
		t.Fatalf("URL is %q, want the suite-wide variable's", cfg.ServerURL())
	}
}

// Logging out must take the token and nothing else: that file also holds the
// sync settings, and clobbering them destroys state nobody asked to reset.
// Revocation runs against a local server so the best-effort call is exercised
// without reaching for a name that cannot resolve, and running logout again
// on an already-logged-out machine is not an error — the state a user in a
// hurry on a borrowed machine is most likely to be in.
func TestLogoutClearsTheTokenAndKeepsEverythingElse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.TokenEnv, "")

	revoked := make(chan string, 2)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revoked <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(api.Close)

	stored := &config.MyceliumConfig{
		URL: api.URL, Token: "live-token",
		Machine: "lucy", Space: "space-1", UsageToken: "usage-token",
		RuleOrder: []string{"one", "two"}, Agents: []string{"claude", "codex"},
	}
	if err := config.SaveMyceliumConfig(stored); err != nil {
		t.Fatal(err)
	}

	if err := logoutCmd.RunE(logoutCmd, nil); err != nil {
		t.Fatalf("logout: %v", err)
	}

	select {
	case got := <-revoked:
		if got != "Bearer live-token" {
			t.Fatalf("the server was asked to revoke %q", got)
		}
	default:
		t.Fatal("the session was cleared locally but never revoked on the server")
	}

	after, err := config.LoadMyceliumConfig()
	if err != nil {
		t.Fatal(err)
	}
	if after.Token != "" {
		t.Fatal("the token survived the logout")
	}
	stored.Token = ""
	if fmt.Sprint(*after) != fmt.Sprint(*stored) {
		t.Fatalf("logout changed more than the token:\n got %+v\nwant %+v", *after, *stored)
	}

	if err := logoutCmd.RunE(logoutCmd, nil); err != nil {
		t.Fatalf("logout while logged out: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".mycelium.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("the config file is mode %v, want no group or other bits", info.Mode().Perm())
	}
}
