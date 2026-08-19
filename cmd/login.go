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

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/daemon"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
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
	Short: "Authenticate with a Mycelium server and save sync config",
	Long: `Authenticate with a Mycelium server and save sync config.

By default this signs you in through your browser against the server's identity
provider, so a session already open with another Facile tool completes the login
without a second prompt. A server with no identity provider, or a machine with
no browser, falls back to approving the machine from a logged-in Mycelium session
(device authorization). Alternatives:

  mycelium login <url> --token <token>     use a token from the dashboard
  mycelium login <url> --token-stdin       read the token from stdin
  mycelium login <url> --password          authenticate with the server password

The URL may be omitted once MYCELIUM_URL or a previous login has set one.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}

		serverURL := cfg.ServerURL()
		if len(args) == 1 {
			serverURL = args[0]
		}
		serverURL = strings.TrimRight(serverURL, "/")
		if serverURL == "" {
			return fmt.Errorf("no server known — run 'mycelium login <url>' or set %s", config.URLEnv)
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

func finishLogin(cfg *config.MyceliumConfig, serverURL, token, machine string) error {
	cfg.URL = serverURL
	cfg.Token = token
	cfg.Machine = machine
	if err := config.SaveMyceliumConfig(cfg); err != nil {
		return err
	}

	color.Green("Logged in to %s as %s", serverURL, machine)
	fmt.Printf("Config saved to %s\n", config.ConfigPath())

	if loginSpace != "" {
		if err := selectLoginSpace(cfg, loginSpace); err != nil {
			color.Yellow("Space not selected: %v", err)
			fmt.Println("Select later with: mycelium spaces use <name-or-id>")
		}
	}

	if !loginNoDaemon {
		if err := daemon.Install(); err != nil {
			color.Yellow("Background sync not enabled: %v", err)
			fmt.Println("Enable later with: mycelium daemon install")
		} else {
			color.Green("Background sync enabled (every %ds). Disable with: mycelium daemon uninstall", daemon.IntervalSeconds)
		}
	}
	return nil
}

func selectLoginSpace(cfg *config.MyceliumConfig, arg string) error {
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
	loginCmd.Flags().StringVarP(&loginMachine, "machine", "m", "", "Machine name to register (default: config machine or hostname)")
	loginCmd.Flags().BoolVar(&loginNoDaemon, "no-daemon", false, "Skip enabling the background sync service")
	loginCmd.Flags().StringVar(&loginToken, "token", "", "Authenticate with a token from the dashboard")
	loginCmd.Flags().BoolVar(&loginTokenStdin, "token-stdin", false, "Read the token from stdin")
	loginCmd.Flags().BoolVar(&loginPassword, "password", false, "Authenticate with the server password instead of the browser")
	loginCmd.Flags().BoolVar(&loginPasswordStdin, "password-stdin", false, "Read the server password from stdin")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Print the authorization URL instead of opening a browser")
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "Select a space to sync after login (name or id)")
	rootCmd.AddCommand(loginCmd)
}
