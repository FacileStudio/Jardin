package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	hsync "github.com/FacileStudio/Mycelium/internal/sync"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var (
	spacesUseNone bool
	spacesJSON    bool
)

type spaceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func fetchSpaces(cfg *config.MyceliumConfig) ([]spaceInfo, error) {
	if cfg.ServerURL() == "" {
		return nil, fmt.Errorf("sync not configured — run 'mycelium login <url>'")
	}
	req, err := http.NewRequest("GET", cfg.ServerURL()+"/api/spaces", nil)
	if err != nil {
		return nil, err
	}
	if token := cfg.AuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func resolveSpace(spaces []spaceInfo, arg string) (*spaceInfo, error) {
	for i := range spaces {
		if spaces[i].ID == arg {
			return &spaces[i], nil
		}
	}
	var prefixMatches []*spaceInfo
	for i := range spaces {
		if strings.HasPrefix(spaces[i].ID, arg) {
			prefixMatches = append(prefixMatches, &spaces[i])
		}
	}
	if len(prefixMatches) == 1 {
		return prefixMatches[0], nil
	}
	if len(prefixMatches) > 1 {
		return nil, fmt.Errorf("ambiguous space prefix %q", arg)
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
	Use:     "spaces",
	Aliases: []string{"space"},
	Short:   "Manage shared memory spaces",
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
		if spaces == nil {
			spaces = []spaceInfo{}
		}

		currentID := cfg.SpaceID()

		if spacesJSON {
			var selected any = nil
			if currentID != "" {
				selected = currentID
			}
			out := map[string]any{
				"selected": selected,
				"spaces":   spaces,
			}
			data, err := json.Marshal(out)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		marker := " "
		if currentID == "" {
			marker = "*"
		}
		fmt.Printf("%s %-8s %-24s %s\n", marker, "-", "common", ui.Dim("the common tree"))

		for _, s := range spaces {
			m := " "
			if currentID == s.ID {
				m = "*"
			}
			fmt.Printf("%s %-8s %-24s %s\n", m, shortID(s.ID), s.Name, ui.Dim(s.Role))
		}
		return nil
	},
}

var spacesCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the currently active space",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}
		spaceID := cfg.SpaceID()
		if spaceID == "" {
			if spacesJSON {
				out := map[string]any{"selected": nil}
				data, _ := json.Marshal(out)
				fmt.Println(string(data))
				return nil
			}
			fmt.Println("common")
			return nil
		}
		spaces, err := fetchSpaces(cfg)
		if err == nil {
			for _, s := range spaces {
				if s.ID == spaceID {
					if spacesJSON {
						out := map[string]any{"selected": s.ID, "name": s.Name}
						data, _ := json.Marshal(out)
						fmt.Println(string(data))
						return nil
					}
					fmt.Printf("%s (%s)\n", s.Name, shortID(s.ID))
					return nil
				}
			}
		}
		if spacesJSON {
			out := map[string]any{"selected": spaceID}
			data, _ := json.Marshal(out)
			fmt.Println(string(data))
			return nil
		}
		fmt.Println(shortID(spaceID))
		return nil
	},
}

var spacesUseCmd = &cobra.Command{
	Use:     "use <name-or-id>",
	Aliases: []string{"switch", "select", "set"},
	Short:   "Select the space this machine syncs (or --none for common)",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}

		if spacesUseNone || (len(args) == 1 && (args[0] == "common" || args[0] == "personal")) {
			if err := setSpace(cfg, ""); err != nil {
				return err
			}
			if spacesJSON {
				out := map[string]any{"selected": nil}
				data, _ := json.Marshal(out)
				fmt.Println(string(data))
				return nil
			}
			ui.Success("Now syncing the common tree")
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
		if spacesJSON {
			out := map[string]any{"selected": space.ID}
			data, _ := json.Marshal(out)
			fmt.Println(string(data))
			return nil
		}
		ui.Success("Now syncing space %s (%s)", space.Name, shortID(space.ID))
		ui.Hint("`mycelium spaces use common` goes back to the common tree")
		return nil
	},
}

func init() {
	spacesCmd.PersistentFlags().BoolVar(&spacesJSON, "json", false, "Output as JSON")
	spacesUseCmd.Flags().BoolVar(&spacesUseNone, "none", false, "Clear the space and sync the common tree")
	spacesCmd.AddCommand(spacesListCmd)
	spacesCmd.AddCommand(spacesCurrentCmd)
	spacesCmd.AddCommand(spacesUseCmd)
	rootCmd.AddCommand(spacesCmd)
}
