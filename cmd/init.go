package cmd

import (
	"github.com/FacileStudio/Mycelium/internal/cell"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/journal"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize mycelium data directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cell.Init(); err != nil {
			return err
		}
		if err := journal.Init(config.DataDir()); err != nil {
			ui.Warn("history not started: %v", err)
		}
		color.Green("Initialized at %s", config.DataDir())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
