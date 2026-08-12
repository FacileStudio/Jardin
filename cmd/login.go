package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/daemon"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	loginMachine       string
	loginNoDaemon      bool
	loginToken         string
	loginTokenStdin    bool
	loginPassword      bool
	loginPasswordStdin bool
	loginNoBrowser     bool
	loginSpace         string
)

var loginCmd = &cobra.Command{
	Use:   "login [url]",
	Short: "Authenticate with a Jardin server and save sync config",
	Long: `Authenticate with a Jardin server and save sync config.

By default this signs you in through your browser against the server's identity
provider, so a session already open with another Facile tool completes the login
without a second prompt. A server with no identity provider, or a machine with
no browser, falls back to approving the machine from a logged-in Jardin session
(device authorization). Alternatives:

  jardin login <url> --token <token>     use a token from the dashboard
  jardin login <url> --token-stdin       read the token from stdin
  jardin login <url> --password          authenticate with the server password

The URL may be omitted once JARDIN_URL or a previous login has set one.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}

		serverURL := cfg.ServerURL()
		if len(args) == 1 {
			serverURL = args[0]
		}
		serverURL = strings.TrimRight(serverURL, "/")
		if serverURL == "" {
			return fmt.Errorf("no server known — run 'jardin login <url>' or set %s", config.URLEnv)
		}

		machine := loginMachine
		if machine == "" {
			machine = cfg.Machine
		}
		if machine == "" {
			machine, _ = os.Hostname()
		}

		var token string
		switch {
		case loginToken != "" || loginTokenStdin:
			token = loginToken
			if loginTokenStdin {
				raw, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read token: %w", err)
				}
				token = strings.TrimSpace(string(raw))
			}
			if token == "" {
				return fmt.Errorf("empty token")
			}
			if err := validateToken(serverURL, token); err != nil {
				return err
			}
		case loginPassword || loginPasswordStdin:
			token, err = passwordLogin(serverURL, machine)
			if err != nil {
				return err
			}
		default:
			if browserAvailable() && serverOffersSSO(serverURL) {
				token, err = ssoLogin(serverURL)
			} else {
				token, err = deviceLogin(serverURL, machine)
			}
			if err != nil {
				return err
			}
		}

		return finishLogin(cfg, serverURL, token, machine)
	},
}

func finishLogin(cfg *config.JardinConfig, serverURL, token, machine string) error {
	cfg.URL = serverURL
	cfg.Token = token
	cfg.Machine = machine
	if err := config.SaveJardinConfig(cfg); err != nil {
		return err
	}

	color.Green("Logged in to %s as %s", serverURL, machine)
	fmt.Printf("Config saved to %s\n", config.ConfigPath())

	if loginSpace != "" {
		if err := selectLoginSpace(cfg, loginSpace); err != nil {
			color.Yellow("Space not selected: %v", err)
			fmt.Println("Select later with: jardin spaces use <name-or-id>")
		}
	}

	if !loginNoDaemon {
		if err := daemon.Install(); err != nil {
			color.Yellow("Background sync not enabled: %v", err)
			fmt.Println("Enable later with: jardin daemon install")
		} else {
			color.Green("Background sync enabled (every %ds). Disable with: jardin daemon uninstall", daemon.IntervalSeconds)
		}
	}
	return nil
}

func selectLoginSpace(cfg *config.JardinConfig, arg string) error {
	spaces, err := fetchSpaces(cfg)
	if err != nil {
		return err
	}
	space, err := resolveSpace(spaces, arg)
	if err != nil {
		return err
	}
	if err := setSpace(cfg, space.ID); err != nil {
		return err
	}
	color.Green("Syncing space %s (%s)", space.Name, space.ID)
	return nil
}

// deviceLogin runs the RFC 8628 device-authorization flow against an API
// that offers no browser sign-in (or when the machine has no browser). The
// poll loop treats two statuses as terminal — denied, expired or already
// consumed (400/403) — and everything else, pending (202), rate-limited (429)
// or a transient blip, as keep-waiting.
func deviceLogin(serverURL, machine string) (string, error) {
	status, body, err := postJSON(serverURL+"/api/auth/device/start", map[string]string{"machine": machine})
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("could not start authorization: %s", strings.TrimSpace(string(body)))
	}

	var start struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		VerifyURL  string `json:"verification_uri_complete"`
		Interval   int    `json:"interval"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &start); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	if start.Interval <= 0 {
		start.Interval = 5
	}

	fmt.Println()
	fmt.Println("To authorize this machine, open this URL in your browser:")
	color.Cyan("  %s", start.VerifyURL)
	fmt.Printf("\n  and confirm the code: ")
	color.New(color.Bold).Printf("%s\n\n", start.UserCode)
	if !loginNoBrowser && term.IsTerminal(int(os.Stdout.Fd())) {
		openBrowser(start.VerifyURL)
	}
	fmt.Print("Waiting for approval")

	deadline := time.Now().Add(time.Duration(start.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(start.Interval) * time.Second)
		fmt.Print(".")

		status, body, err := postJSON(serverURL+"/api/auth/device/poll", map[string]string{"device_code": start.DeviceCode})
		if err != nil {
			continue
		}
		switch status {
		case http.StatusOK:
			var res struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(body, &res); err != nil {
				return "", fmt.Errorf("invalid response: %w", err)
			}
			fmt.Println()
			return res.Token, nil
		case http.StatusBadRequest, http.StatusForbidden:
			fmt.Println()
			return "", fmt.Errorf("authorization failed: %s", strings.TrimSpace(string(body)))
		default:
			continue
		}
	}
	fmt.Println()
	return "", fmt.Errorf("authorization timed out — run `jardin login` again")
}

// ssoWait is how long the listener stays open for the browser to come back.
// Long enough to type a password and answer a second factor, short enough that
// a closed tab does not leave a socket open all afternoon.
const ssoWait = 3 * time.Minute

// errCallbackMismatch is a hard stop, never a retry: a callback carrying a code
// under the wrong nonce came from something other than this login.
var errCallbackMismatch = errors.New("the sign-in callback did not match this login attempt — run 'jardin login' again")

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
			http.Error(w, "The callback did not match this login attempt. Run `jardin login` again.", http.StatusBadRequest)
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
		result = outcome{err: fmt.Errorf("timed out waiting for the browser — run 'jardin login' again")}
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

func passwordLogin(serverURL, machine string) (string, error) {
	var password string
	if loginPasswordStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		password = strings.TrimRight(string(raw), "\r\n")
	} else {
		fmt.Print("Password: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		password = string(raw)
	}

	status, body, err := postJSON(serverURL+"/api/auth/login", map[string]string{"password": password, "machine": machine})
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", strings.TrimSpace(string(body)))
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("invalid response: %w", err)
	}
	return result.Token, nil
}

func validateToken(serverURL, token string) error {
	req, err := http.NewRequest("GET", serverURL+"/api/status", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token rejected by %s", serverURL)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

func postJSON(url string, payload any) (int, []byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "cmd", []string{"/c", "start", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

func init() {
	loginCmd.Flags().StringVarP(&loginMachine, "machine", "m", "", "machine name to register (default: config machine or hostname)")
	loginCmd.Flags().BoolVar(&loginNoDaemon, "no-daemon", false, "skip enabling the background sync service")
	loginCmd.Flags().StringVar(&loginToken, "token", "", "authenticate with a token from the dashboard")
	loginCmd.Flags().BoolVar(&loginTokenStdin, "token-stdin", false, "read the token from stdin")
	loginCmd.Flags().BoolVar(&loginPassword, "password", false, "authenticate with the server password instead of the browser")
	loginCmd.Flags().BoolVar(&loginPasswordStdin, "password-stdin", false, "read the server password from stdin")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "print the authorization URL instead of opening a browser")
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "select a space to sync after login (name or id)")
	rootCmd.AddCommand(loginCmd)
}
