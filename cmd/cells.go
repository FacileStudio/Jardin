package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var cellsCmd = &cobra.Command{
	Use:   "cells",
	Short: "List all cells",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}

		if len(cfg.Cells) == 0 {
			fmt.Println("No cells. Run 'ruche init <name>' to create one.")
			return nil
		}

		for _, c := range cfg.Cells {
			marker := "  "
			if c.Name == cfg.ActiveCell {
				marker = color.GreenString("* ")
			}
			fmt.Printf("%s%s\t%s\n", marker, c.Name, c.Path)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cellsCmd)
}
