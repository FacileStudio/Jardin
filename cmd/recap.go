package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/flow"
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
		recap := joinSections(sessions.Recap(config.DataDir(), project, time.Now()), flowRecap())
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

func joinSections(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, strings.TrimRight(p, "\n"))
		}
	}
	return strings.Join(kept, "\n\n")
}

func flowRecap() string {
	flows, err := flow.List()
	if err != nil || len(flows) == 0 {
		return ""
	}
	width := flowNameWidth(flows)
	lines := make([]string, 0, len(flows)+2)
	lines = append(lines, "Flows on this machine (run one instead of re-deriving it):")
	for _, f := range flows {
		lines = append(lines, flowRecapLine(f, width))
	}
	lines = append(lines, "Run with: jardin flow run <name>")
	return strings.Join(lines, "\n")
}

func init() {
	recapCmd.Flags().BoolVar(&recapHook, "hook", false, "read hook JSON from stdin, emit hookSpecificOutput")
	rootCmd.AddCommand(recapCmd)
}
