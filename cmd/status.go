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
	Short: "Show machine, sync state, and content summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}

		color.New(color.Bold).Printf("Machine: ")
		if cfg.Machine != "" {
			fmt.Println(cfg.Machine)
		} else {
			fmt.Println("not set")
		}

		color.New(color.Bold).Printf("Sync:    ")
		if cfg.URL != "" {
			fmt.Println(cfg.URL)
		} else {
			fmt.Println("not configured")
		}

		rules, _ := cell.ListRules()
		skills, _ := cell.ListSkills()

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
