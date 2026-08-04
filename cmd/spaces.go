package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	hsync "github.com/FacileStudio/Mycelium/internal/sync"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var spacesUseNone bool

type spaceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func fetchSpaces(cfg *config.MyceliumConfig) ([]spaceInfo, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("sync not configured — run 'mycelium login <url>'")
	}
	req, err := http.NewRequest("GET", cfg.URL+"/api/spaces", nil)
	if err != nil {
		return nil, err
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("spaces: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Spaces []spaceInfo `json:"spaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return out.Spaces, nil
}

// resolveSpace matches by exact id first, then case-insensitively by name.
func resolveSpace(spaces []spaceInfo, arg string) (*spaceInfo, error) {
	for i := range spaces {
		if spaces[i].ID == arg {
			return &spaces[i], nil
		}
	}
	for i := range spaces {
		if strings.EqualFold(spaces[i].Name, arg) {
			return &spaces[i], nil
		}
	}
	return nil, fmt.Errorf("no space matching %q — run 'mycelium spaces list'", arg)
}

func setSpace(cfg *config.MyceliumConfig, spaceID string) error {
	if cfg.Space == spaceID {
		return nil
	}
	cfg.Space = spaceID
	if err := config.SaveMyceliumConfig(cfg); err != nil {
		return err
	}
	return hsync.ResetBase(config.DataDir())
}

var spacesCmd = &cobra.Command{
	Use:   "spaces",
	Short: "Manage shared memory spaces",
}

var spacesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List spaces available to this machine",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}
		spaces, err := fetchSpaces(cfg)
		if err != nil {
			return err
		}
		if len(spaces) == 0 {
			fmt.Println("No spaces available.")
			return nil
		}
		for _, s := range spaces {
			color.New(color.FgCyan).Printf("%s  ", s.ID)
			color.New(color.Bold).Printf("%s", s.Name)
			fmt.Printf("  (%s)", s.Role)
			if cfg.Space == s.ID {
				color.Green("  [current]")
			} else {
				fmt.Println()
			}
		}
		return nil
	},
}

var spacesUseCmd = &cobra.Command{
	Use:   "use <name-or-id>",
	Short: "Select the space this machine syncs (or --none for common)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}

		if spacesUseNone || (len(args) == 1 && args[0] == "common") {
			if err := setSpace(cfg, ""); err != nil {
				return err
			}
			color.Green("Now syncing the common tree.")
			return nil
		}
		if len(args) == 0 {
			return fmt.Errorf("space name or id required (or --none for the common tree)")
		}

		spaces, err := fetchSpaces(cfg)
		if err != nil {
			return err
		}
		space, err := resolveSpace(spaces, args[0])
		if err != nil {
			return err
		}
		if err := setSpace(cfg, space.ID); err != nil {
			return err
		}
		color.Green("Now syncing space %s (%s).", space.Name, space.ID)
		return nil
	},
}

func init() {
	spacesUseCmd.Flags().BoolVar(&spacesUseNone, "none", false, "clear the space and sync the common tree")
	spacesCmd.AddCommand(spacesListCmd)
	spacesCmd.AddCommand(spacesUseCmd)
	rootCmd.AddCommand(spacesCmd)
}
