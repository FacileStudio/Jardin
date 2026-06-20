package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "hive",
	Short: "Shared agent brain across AI coding agents and machines",
	Long:  "Hive manages a canonical source of truth for agent memory, rules, and skills. It generates per-agent configs via thin adapters and syncs across machines via git.",
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().String("cell", "", "override active cell for this command")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
