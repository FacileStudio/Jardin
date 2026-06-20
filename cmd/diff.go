package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/FacileStudio/Hive/internal/adapter"
	"github.com/FacileStudio/Hive/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <agent>",
	Short: "Preview what install would change",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadHiveConfig()
		if err != nil {
			return err
		}

		a, err := adapter.Get(args[0])
		if err != nil {
			return err
		}

		input, err := buildInput(cfg)
		if err != nil {
			return err
		}

		out, err := a.Generate(*input)
		if err != nil {
			return err
		}

		anyDiff := false
		for path, newContent := range out.Files {
			existing, err := os.ReadFile(path)
			if err != nil {
				color.Yellow("+ %s (new file)", path)
				anyDiff = true
				continue
			}

			if string(existing) == newContent {
				continue
			}

			anyDiff = true
			color.Yellow("~ %s (modified)", path)

			oldLines := strings.Split(string(existing), "\n")
			newLines := strings.Split(newContent, "\n")
			printSimpleDiff(oldLines, newLines)
		}

		if !anyDiff {
			color.Green("No changes for %s.", a.Name())
		}

		return nil
	},
}

func printSimpleDiff(old, new []string) {
	maxLen := len(old)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(old) {
			oldLine = old[i]
		}
		if i < len(new) {
			newLine = new[i]
		}
		if oldLine != newLine {
			if oldLine != "" {
				color.Red("- %s", oldLine)
			}
			if newLine != "" {
				color.Green("+ %s", newLine)
			}
		}
	}
	fmt.Println()
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
