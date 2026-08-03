package cmd

import (
	"github.com/FacileStudio/Jardin/internal/cell"
	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize jardin data directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cell.Init(); err != nil {
			return err
		}
		color.Green("Initialized at %s", config.DataDir())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
