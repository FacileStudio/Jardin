package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "ruche",
	Short: "Shared agent memory across AI coding agents and machines",
	Long:  "Ruche manages a canonical source of truth for agent memory, rules, and skills. It generates per-agent configs via thin adapters and syncs across machines.",
}

func init() {
	rootCmd.Version = version
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
