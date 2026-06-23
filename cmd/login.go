package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/FacileStudio/Ruche/internal/daemon"
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
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Authenticate with a Ruche server and save sync config",
	Long: `Authenticate with a Ruche server and save sync config.

By default this opens your browser to approve the machine from a logged-in
Ruche session (device authorization). Alternatives:

  ruche login <url> --token <token>     use a token from the dashboard
  ruche login <url> --token-stdin       read the token from stdin
  ruche login <url> --password          authenticate with the server password`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := strings.TrimRight(args[0], "/")

		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
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
			token, err = deviceLogin(serverURL, machine)
			if err != nil {
				return err
			}
		}

		return finishLogin(cfg, serverURL, token, machine)
	},
}

func finishLogin(cfg *config.RucheConfig, serverURL, token, machine string) error {
	cfg.URL = serverURL
	cfg.Token = token
	cfg.Machine = machine
	if err := config.SaveRucheConfig(cfg); err != nil {
		return err
	}

	color.Green("Logged in to %s as %s", serverURL, machine)
	fmt.Printf("Config saved to %s\n", config.ConfigPath())

	if !loginNoDaemon {
		if err := daemon.Install(); err != nil {
			color.Yellow("Background sync not enabled: %v", err)
			fmt.Println("Enable later with: ruche daemon install")
		} else {
			color.Green("Background sync enabled (every %ds). Disable with: ruche daemon uninstall", daemon.IntervalSeconds)
		}
	}
	return nil
}

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
			// Terminal: denied, expired, or already consumed.
			fmt.Println()
			return "", fmt.Errorf("authorization failed: %s", strings.TrimSpace(string(body)))
		default:
			// Pending (202), rate-limited (429), or a transient blip — keep waiting.
			continue
		}
	}
	fmt.Println()
	return "", fmt.Errorf("authorization timed out — run `ruche login` again")
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
	rootCmd.AddCommand(loginCmd)
}
