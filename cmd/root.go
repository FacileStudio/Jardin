package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/Mycelium/internal/ui"
)

var version = "dev"
var flagSpace string

var rootCmd = &cobra.Command{
	Use:   "mycelium",
	Short: "Shared agent memory across AI coding agents and machines",
	Long:  "Mycelium manages a canonical source of truth for agent memory, rules, and skills. It generates per-agent configs via thin adapters and syncs across machines.",
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().StringVar(&flagSpace, "space", "", "Target memory space ID or name")
	cobra.OnInitialize(func() {
		if v, _ := rootCmd.PersistentFlags().GetBool("no-color"); v {
			ui.DisableColor()
		}
		if flagSpace != "" {
			_ = os.Setenv("MYCELIUM_SPACE", flagSpace)
		}
	})
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
