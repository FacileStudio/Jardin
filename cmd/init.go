package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/cell"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Create a new cell",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cellPath, err := cell.Init(name)
		if err != nil {
			return err
		}

		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cfg.AddCell(name, cellPath)
		if cfg.ActiveCell == "" {
			cfg.ActiveCell = name
		}
		if err := config.SaveRucheConfig(cfg); err != nil {
			return err
		}

		color.Green("Cell %q created at %s", name, cellPath)
		if cfg.ActiveCell == name {
			fmt.Println("Set as active cell.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
