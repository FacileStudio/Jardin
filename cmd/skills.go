package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Ruche/internal/cell"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage skills",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List skill files",
	RunE: func(cmd *cobra.Command, args []string) error {
		skills, err := cell.ListSkills()
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
		name := args[0]
		path := filepath.Join(config.SkillsDir(), name+".md")
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("skill %q already exists", name)
		}

		os.MkdirAll(config.SkillsDir(), 0755)
		content := fmt.Sprintf("---\nname: %s\ndescription: \"\"\ntriggers: [\"/%s\"]\n---\n\n# %s\n", name, name, name)

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
