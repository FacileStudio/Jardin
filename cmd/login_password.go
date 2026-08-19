package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
)

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
