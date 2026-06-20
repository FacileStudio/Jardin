package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/memory"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage memory",
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := memory.Search(config.MemoryDir(), args[0])
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

var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Show memory index",
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := memory.ReadIndex(config.MemoryDir())
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	},
}

func init() {
	memoryCmd.AddCommand(memorySearchCmd)
	memoryCmd.AddCommand(memoryIndexCmd)
	rootCmd.AddCommand(memoryCmd)
}
