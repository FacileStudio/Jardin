package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Authenticate with a Ruche server and save sync config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := strings.TrimRight(args[0], "/")

		fmt.Print("Password: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password := string(raw)

		body, _ := json.Marshal(map[string]string{"password": password})
		resp, err := http.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			msg, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("login failed: %s", string(msg))
		}

		var result struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("invalid response: %w", err)
		}

		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cfg.SyncURL = serverURL
		cfg.SyncToken = result.Token
		if err := config.SaveRucheConfig(cfg); err != nil {
			return err
		}

		color.Green("Logged in to %s", serverURL)
		fmt.Printf("Config saved to %s\n", config.ConfigPath())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
