package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Hive/internal/cell"
	"github.com/FacileStudio/Hive/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage cell skills",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List skill files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadHiveConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		skills, err := cell.ListSkills(cellPath)
		if err != nil {
			return err
		}

		if len(skills) == 0 {
			fmt.Println("No skills.")
			return nil
		}

		for _, s := range skills {
			fmt.Println(s)
		}
		return nil
	},
}

var skillsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Scaffold a new skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadHiveConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		name := args[0]
		path := filepath.Join(cellPath, "skills", name+".md")
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("skill %q already exists", name)
		}

		content := fmt.Sprintf(`---
name: %s
description: ""
triggers: ["/%s"]
---

# %s
`, name, name, name)

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}

		color.Green("Skill %q created at %s", name, path)
		return nil
	},
}

func init() {
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsAddCmd)
	rootCmd.AddCommand(skillsCmd)
}
