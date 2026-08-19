package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

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
