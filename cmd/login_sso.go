package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// ssoWait is how long the listener stays open for the browser to come back.
// Long enough to type a password and answer a second factor, short enough that
// a closed tab does not leave a socket open all afternoon.
const ssoWait = 3 * time.Minute

// errCallbackMismatch is a hard stop, never a retry: a callback carrying a code
// under the wrong nonce came from something other than this login.
var errCallbackMismatch = errors.New("the sign-in callback did not match this login attempt — run 'mycelium login' again")

// browserAvailable reports whether opening a URL can plausibly reach a human.
// A CI job or an SSH session on a headless box has no browser to redirect back
// from, which is exactly the case the device flow exists for.
func browserAvailable() bool {
	if loginNoBrowser || !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// serverOffersSSO asks discovery rather than making the user find out by
// having a browser open on a server with no identity provider.
func serverOffersSSO(serverURL string) bool {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(serverURL + "/api/auth/config")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var discovery struct {
		OIDCEnabled bool `json:"oidc_enabled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return false
	}
	return discovery.OIDCEnabled
}

// ssoLogin signs in through the browser and returns a session token.
//
// The whole exchange belongs to the server: the CLI never sees the identity
// provider, never handles a password, and never holds anything that is worth
// something on its own. It opens a loopback port, sends the browser to the API
// with that port attached, and the API — once the provider has done its part —
// redirects back with a one-time code good for sixty seconds.
//
// The loopback listener asks the kernel for a free port, so two shells can
// log in at the same time without agreeing on anything. The nonce is what
// lets the listener tell its own callback from one somebody else sent:
// without it any local process that guesses the port can hand us a code of
// its choosing and we would exchange it.
func ssoLogin(serverURL string) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("cannot open a loopback port to receive the login: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	nonce, err := loginNonce()
	if err != nil {
		listener.Close()
		return "", err
	}

	authURL := fmt.Sprintf("%s/api/auth/oidc?flow=cli&port=%d&cli_state=%s", serverURL, port, nonce)
	fmt.Println()
	fmt.Println("To sign in, open this URL in your browser:")
	color.Cyan("  %s", authURL)
	fmt.Println()
	openBrowser(authURL)
	fmt.Print("Waiting for the browser")

	code, err := awaitLoginCode(listener, nonce)
	fmt.Println()
	if err != nil {
		return "", err
	}

	status, body, err := postJSON(serverURL+"/api/auth/oidc/exchange", map[string]string{"code": code})
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("sign-in failed: %s", strings.TrimSpace(string(body)))
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("the server returned no token")
	}
	return result.Token, nil
}

// awaitLoginCode serves the one redirect the API sends the browser to, and
// keeps listening through anything else: a browser asks for /favicon.ico
// unprompted, and failing a login over that is a bug nobody can diagnose.
// It shuts the listener down rather than closing it, so the page the browser
// is still reading finishes arriving before the socket goes away.
func awaitLoginCode(listener net.Listener, nonce string) (string, error) {
	type outcome struct {
		code string
		err  error
	}
	done := make(chan outcome, 1)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Not the login redirect.", http.StatusNotFound)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(nonce)) != 1 {
			http.Error(w, "The callback did not match this login attempt. Run `mycelium login` again.", http.StatusBadRequest)
			select {
			case done <- outcome{err: errCallbackMismatch}:
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Signed in. You can close this tab and go back to your terminal.")
		select {
		case done <- outcome{code: code}:
		default:
		}
	})}
	go server.Serve(listener)

	var result outcome
	select {
	case result = <-done:
	case <-time.After(ssoWait):
		result = outcome{err: fmt.Errorf("timed out waiting for the browser — run 'mycelium login' again")}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	return result.code, result.err
}

func loginNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot generate a login nonce: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
