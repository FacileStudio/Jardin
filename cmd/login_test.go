package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/FacileStudio/porte/loopback"

	"github.com/FacileStudio/Mycelium/internal/config"
)

// The default hand-off names the loopback port this CLI is listening on. The
// mount point is the API's decision rather than porte's, and a login URL built
// against the wrong one fails silently: the browser completes an ordinary web
// login, the person lands on the dashboard, and the listener waits out its
// timeout for a redirect nobody sends.
func TestTheLoopbackLoginURLNamesTheMountAndThePort(t *testing.T) {
	listener, err := loopback.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	target, err := url.Parse(listener.LoginURL("https://mycelium.facile.studio", porteMount, "nonce"))
	if err != nil {
		t.Fatalf("LoginURL did not produce a URL: %v", err)
	}
	if target.Path != "/api/auth/oidc" {
		t.Fatalf("path is %q, want /api/auth/oidc", target.Path)
	}
	if got := target.Query().Get("port"); got != strconv.Itoa(listener.Port()) {
		t.Fatalf("port is %q, want the bound port %d", got, listener.Port())
	}
}

// --no-listener exists for the terminal whose browser is on another machine,
// and the port is the whole of the difference. With one the server redirects
// the code to 127.0.0.1, which on a copied URL is the wrong machine's loopback;
// without one it shows the code on the page for the person to carry back.
func TestThePasteLoginURLCarriesNoPort(t *testing.T) {
	target, err := url.Parse(pasteLoginURL("https://mycelium.facile.studio/"))
	if err != nil {
		t.Fatalf("pasteLoginURL did not produce a URL: %v", err)
	}
	if target.Path != "/api/auth/oidc" {
		t.Fatalf("path is %q, want /api/auth/oidc", target.Path)
	}
	if target.Query().Get("flow") != "cli" {
		t.Fatalf("flow is %q, want cli", target.Query().Get("flow"))
	}
	if target.Query().Has("port") {
		t.Fatal("the paste URL names a port, so the server will redirect the code to a machine that is not listening")
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
