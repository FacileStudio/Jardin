package cmd

import (
	"fmt"
	"strings"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/memory"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var memorySearchLimit int

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage memory",
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search memory, best match first",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := memory.Search(config.MemoryDir(), strings.Join(args, " "))
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("No results.")
			return nil
		}
		shown := results
		if memorySearchLimit > 0 && len(shown) > memorySearchLimit {
			shown = shown[:memorySearchLimit]
		}
		for _, r := range shown {
			color.New(color.FgCyan).Printf("%s:%d ", r.Path, r.Line)
			fmt.Println(r.Content)
		}
		if len(shown) < len(results) {
			fmt.Printf("\n%d more match(es); raise --limit to see them.\n", len(results)-len(shown))
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
	memorySearchCmd.Flags().IntVar(&memorySearchLimit, "limit", 20, "maximum results to print")
	memoryCmd.AddCommand(memorySearchCmd)
	memoryCmd.AddCommand(memoryIndexCmd)
	rootCmd.AddCommand(memoryCmd)
}
