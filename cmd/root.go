package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/Jardin/internal/ui"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "jardin",
	Short: "Shared agent memory across AI coding agents and machines",
	Long:  "Jardin manages a canonical source of truth for agent memory, rules, and skills. It generates per-agent configs via thin adapters and syncs across machines.",
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	cobra.OnInitialize(func() {
		if v, _ := rootCmd.PersistentFlags().GetBool("no-color"); v {
			ui.DisableColor()
		}
	})
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
