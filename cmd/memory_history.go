package cmd

import (
	"fmt"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/journal"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// The history is for a human asking what happened to a page. Nothing instructs
// an agent to run these, and no help text here names the storage underneath:
// agents reason about sync and never about how it is kept.

var memoryLogLimit int

var memoryLogCmd = &cobra.Command{
	Use:   "log [path]",
	Short: "Show what changed in memory, newest first",
	Long: "Show what changed in memory, newest first.\n\n" +
		"Pass a page to narrow it, spelled the way search reports it: " +
		"'mycelium memory log conventions/no-slop.md'. Each line starts with the " +
		"ref you hand to 'diff' and 'revert'.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := journal.Log(config.DataDir(), firstArg(args), memoryLogLimit)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No history yet.")
			return nil
		}
		for _, e := range entries {
			color.New(color.FgCyan).Printf("%s ", e.Ref)
			fmt.Printf("%s  %-10s %s\n", e.When.Format("2006-01-02 15:04"), e.Machine, e.Message)
		}
		return nil
	},
}

var memoryDiffCmd = &cobra.Command{
	Use:   "diff <ref> [path]",
	Short: "Show what memory has done since a recorded state",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		patch, err := journal.Diff(config.DataDir(), args[0], firstArg(args[1:]))
		if err != nil {
			return err
		}
		if strings.TrimSpace(patch) == "" {
			fmt.Println("Nothing changed.")
			return nil
		}
		fmt.Print(patch)
		return nil
	},
}

var memoryRevertCmd = &cobra.Command{
	Use:   "revert <ref> [path]",
	Short: "Put memory back the way it was at a recorded state",
	Long: "Put memory back the way it was at a recorded state.\n\n" +
		"With a path, only that page or directory moves. With no path the whole " +
		"authored tree does, which is the answer to a sync that deleted more than " +
		"it should have. The state being replaced is recorded first, so a revert " +
		"to the wrong ref is itself revertible.\n\n" +
		"Nothing reaches the other machines until the next 'mycelium sync'.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := firstArg(args[1:])
		if err := journal.Revert(config.DataDir(), args[0], path); err != nil {
			return err
		}
		ui.Success("Reverted %s to %s", revertLabel(path), args[0])
		ui.Hint("Run 'mycelium sync' to send this to the other machines.")
		return nil
	},
}

func revertLabel(path string) string {
	if strings.TrimSpace(path) == "" {
		return "memory"
	}
	return path
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func init() {
	memoryLogCmd.Flags().IntVar(&memoryLogLimit, "limit", 20, "Maximum entries to print")
	memoryCmd.AddCommand(memoryLogCmd)
	memoryCmd.AddCommand(memoryDiffCmd)
	memoryCmd.AddCommand(memoryRevertCmd)
}
