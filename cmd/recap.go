package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/sessions"
	"github.com/spf13/cobra"
)

var recapHook bool

var recapCmd = &cobra.Command{
	Use:    "recap",
	Short:  "Project session recap for agent context injection",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if recapHook {
			var input struct {
				Cwd string `json:"cwd"`
			}
			data, _ := io.ReadAll(os.Stdin)
			if json.Unmarshal(data, &input) == nil && input.Cwd != "" {
				cwd = input.Cwd
			}
		}

		project := sessions.ResolveProject(cwd)
		recap := sessions.Recap(config.DataDir(), project, time.Now())
		if recap == "" {
			return nil
		}
		if !recapHook {
			fmt.Println(recap)
			return nil
		}
		out := map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": recap,
			},
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	},
}

func init() {
	recapCmd.Flags().BoolVar(&recapHook, "hook", false, "read hook JSON from stdin, emit hookSpecificOutput")
	rootCmd.AddCommand(recapCmd)
}
