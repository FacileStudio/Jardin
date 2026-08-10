package cmd

import (
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/ui"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the stored server session",
	Long: `Forget the stored server session.

Revokes the token on the server when it can reach it, then clears it from the
config file. Everything else in that file — the server URL, the machine name,
the selected space, the rule order, the agent list and the Anthropic usage
token — is left alone. It is not an error to run this when not logged in.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}

		if cfg.Token == "" {
			ui.Success("Already logged out.")
		} else {
			revokeSession(cfg.URL, cfg.Token)
			// Read-modify-write: this file holds sync settings beside
			// the token, and serializing a fresh struct would erase them.
			cfg.Token = ""
			if err := config.SaveMyceliumConfig(cfg); err != nil {
				return err
			}
			ui.Success("Logged out. The session token was removed from %s.", config.ConfigPath())
		}

		if os.Getenv(config.TokenEnv) != "" {
			ui.Warn("%s is still set in this environment and will be used instead.", config.TokenEnv)
		}
		return nil
	},
}

// revokeSession is best effort. An unreachable server must not stop the
// credential leaving this machine, which is the half the user asked for.
func revokeSession(serverURL, token string) {
	if serverURL == "" {
		return
	}
	req, err := http.NewRequest("POST", serverURL+"/api/auth/logout", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ui.Warn("Could not reach %s to revoke the session: %v", serverURL, err)
		return
	}
	resp.Body.Close()
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
