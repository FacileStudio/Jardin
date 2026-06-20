package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/FacileStudio/Ruche/internal/brain"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var brainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Manage cell brain (wiki/memory)",
}

var brainSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search brain for a query",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		results, err := brain.Search(filepath.Join(cellPath, "brain"), args[0])
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No results.")
			return nil
		}

		for _, r := range results {
			color.New(color.FgCyan).Printf("%s:%d ", r.Path, r.Line)
			fmt.Println(r.Content)
		}
		return nil
	},
}

var brainIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Show brain index",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		content, err := brain.ReadIndex(filepath.Join(cellPath, "brain"))
		if err != nil {
			return err
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	brainCmd.AddCommand(brainSearchCmd)
	brainCmd.AddCommand(brainIndexCmd)
	rootCmd.AddCommand(brainCmd)
}
