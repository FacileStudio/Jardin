package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/cell"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active cell, machine, and sync state",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}

		if cfg.ActiveCell == "" {
			fmt.Println("No active cell. Run 'ruche init <name>' to get started.")
			return nil
		}

		color.New(color.Bold).Printf("Cell:    ")
		fmt.Println(cfg.ActiveCell)

		color.New(color.Bold).Printf("Machine: ")
		if cfg.Machine != "" {
			fmt.Println(cfg.Machine)
		} else {
			color.Yellow("not set (run 'ruche config machine <name>')")
			fmt.Println()
		}

		color.New(color.Bold).Printf("Sync:    ")
		if cfg.URL != "" {
			fmt.Println(cfg.URL)
		} else {
			fmt.Println("not configured")
		}

		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		cellCfg, err := config.LoadCellConfig(cellPath)
		if err != nil {
			return err
		}

		if cellCfg.PerceptionEndpoint != "" {
			color.New(color.Bold).Printf("Perception: ")
			fmt.Println(cellCfg.PerceptionEndpoint)
		}

		rules, _ := cell.ListRules(cellPath)
		skills, _ := cell.ListSkills(cellPath)

		fmt.Println()
		color.New(color.Bold).Printf("Rules:   ")
		fmt.Printf("%d\n", len(rules))
		color.New(color.Bold).Printf("Skills:  ")
		fmt.Printf("%d\n", len(skills))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
