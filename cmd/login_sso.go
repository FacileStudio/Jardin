package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/porte/loopback"

	"github.com/FacileStudio/Mycelium/internal/browser"
	"github.com/fatih/color"
	"golang.org/x/term"
)

// ssoAppName is the tool the loopback page names. By the time the browser lands
// on 127.0.0.1 it has left the server's domain behind and the address bar
// proves nothing, so the name on the page is all a person has to tell this
// login from any other local process that asked them to sign in.
const ssoAppName = "Mycelium"

// porteMount is the path the API hangs its porte-shaped auth routes off, and
// the reason the login URL is /api/auth/oidc rather than /auth/oidc. A login
// URL built against the wrong mount is not an error anywhere: the browser
// completes an ordinary web login, the person lands on the dashboard, and the
// listener sits out its timeout waiting for a redirect nobody sends.
const porteMount = "/api"

// browserAvailable reports whether opening a URL can plausibly reach a human.
// A CI job or an SSH session on a headless box has no browser to redirect back
// from, which is exactly the case the device flow exists for. The terminal
// check is this command's own: a flow that ends in "waiting for the browser"
// has nobody to wait for when nothing is interactive.
func browserAvailable() bool {
	if loginNoBrowser || !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	return browser.Available()
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
// with that port attached, and the API, once the provider has done its part,
// redirects back with a one-time code good for sixty seconds.
//
// The listener is porte/loopback rather than a copy of it. Every CLI in the
// suite grew its own, and the parts that matter here are the parts that are
// silent when they are wrong: an ephemeral port so two shells can log in at
// once, an exact comparison of the nonce the server echoes back, a refused
// callback that leaves the login open because a browser asks for /favicon.ico
// unprompted, and a shutdown grace so the page finishes arriving. One reviewed
// copy is the only way all of that stays true in every CLI at once.
func ssoLogin(serverURL string) (string, error) {
	if loginNoListener {
		return ssoLoginByPaste(serverURL)
	}

	listener, err := loopback.Listen()
	if err != nil {
		return "", fmt.Errorf("cannot open a loopback port to receive the login: %w", err)
	}
	defer listener.Close()
	listener.AppName = ssoAppName

	state, err := loopback.RandomState()
	if err != nil {
		return "", err
	}

	authURL := listener.LoginURL(serverURL, porteMount, state)
	announceLoginURL(authURL)
	_ = browser.Open(authURL)
	fmt.Print("Waiting for the browser")

	code, err := listener.WaitForCode(context.Background(), state)
	fmt.Println()
	if err != nil {
		if errors.Is(err, loopback.ErrTimeout) {
			return "", fmt.Errorf("timed out waiting for the browser, run 'mycelium login' again")
		}
		return "", err
	}
	return exchangeLoginCode(serverURL, code)
}

// ssoLoginByPaste signs in without a loopback listener, for the machine whose
// browser lives somewhere else.
//
// The login URL carries no port, which is what makes the server show the code
// on the hand-off page instead of redirecting it to 127.0.0.1. That redirect is
// the failure this exists to prevent: a URL printed here and opened on a laptop
// sends the code to the laptop's own port 0, or worse to whatever is listening
// there, and the terminal that started the login waits out its timeout for a
// callback that was never addressed to it.
//
// No nonce is sent either. The nonce exists to prove a callback belongs to this
// login, and there is no callback here: the person carries the code, and the
// server hands it to whoever exchanges it first, once, inside sixty seconds.
func ssoLoginByPaste(serverURL string) (string, error) {
	authURL := pasteLoginURL(serverURL)
	announceLoginURL(authURL)
	if browserAvailable() {
		_ = browser.Open(authURL)
	}

	code, err := readPastedCode()
	if err != nil {
		return "", err
	}
	return exchangeLoginCode(serverURL, code)
}

// pasteLoginURL is the login URL with the loopback port left off, which is the
// single difference between the two hand-offs and the whole of the fix. The
// server reads a missing or unusable port as "this CLI has nowhere to redirect
// to" and renders the code on the page.
func pasteLoginURL(serverURL string) string {
	return strings.TrimRight(serverURL, "/") + porteMount + "/auth/oidc?flow=cli"
}

// announceLoginURL prints the address the login starts at. It is printed on
// every path, browser or not, because a browser that opened somewhere the
// person cannot see leaves the URL as the only way through.
func announceLoginURL(authURL string) {
	fmt.Println()
	fmt.Println("To sign in, open this URL in your browser:")
	color.Cyan("  %s", authURL)
	fmt.Println()
}

// readPastedCode reads the one-time code off the hand-off page from stdin.
//
// An empty line is refused rather than sent: the exchange would answer a
// generic refusal, and "the server refused the code" is a much worse thing to
// read than "nothing was pasted" when the real problem is that the code has
// already expired and the person needs to start again.
func readPastedCode() (string, error) {
	fmt.Print("Paste the code from that page: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("failed to read the login code: %w", err)
	}
	code := strings.TrimSpace(line)
	if code == "" {
		return "", fmt.Errorf("no login code was pasted, the code is good for sixty seconds so run 'mycelium login' again")
	}
	return code, nil
}

// exchangeLoginCode trades the one-time code for the session token, which is
// the only step that is the same whether the code arrived over a loopback
// redirect or in somebody's clipboard.
func exchangeLoginCode(serverURL, code string) (string, error) {
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
