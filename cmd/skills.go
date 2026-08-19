package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/cell"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
			ui.Hint("No skills.")
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
		content := fmt.Sprintf("---\nname: %s\ndescription: \"\"\ntriggers: [\"/%s\"]\nsource: \"\"\n---\n\n# %s\n", name, name, name)

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}

		color.Green("Skill %q created at %s", name, path)
		return nil
	},
}

type skillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers"`
	Source      *string  `yaml:"source"`
}

var skillsValidateAll bool

var skillsValidateCmd = &cobra.Command{
	Use:   "validate [name]",
	Short: "Check skill frontmatter for errors",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !skillsValidateAll && len(args) == 0 {
			return cmd.Help()
		}

		var names []string
		if skillsValidateAll {
			var err error
			names, err = cell.ListSkills()
			if err != nil {
				return err
			}
		} else {
			names = append(names, args[0])
		}

		if len(names) == 0 {
			ui.Hint("No skills to validate.")
			return nil
		}

		ok := true
		for _, name := range names {
			path := filepath.Join(config.SkillsDir(), name+".md")
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Printf("  %s %-20s %s\n", color.RedString("✗"), name, err)
				ok = false
				continue
			}

			errors, warnings := validateSkill(name, data)
			if len(errors) == 0 && len(warnings) == 0 {
				fmt.Printf("  %s %s\n", color.GreenString("✓"), name)
				continue
			}

			if len(errors) > 0 {
				ok = false
				fmt.Printf("  %s %s\n", color.RedString("✗"), name)
			} else {
				fmt.Printf("  %s %s\n", color.YellowString("!"), name)
			}
			for _, e := range errors {
				fmt.Printf("    %s %s\n", color.RedString("error"), e)
			}
			for _, w := range warnings {
				fmt.Printf("    %s %s\n", color.YellowString("warn"), w)
			}
		}

		if !ok {
			return fmt.Errorf("validation failed")
		}
		return nil
	},
}

func validateSkill(filename string, data []byte) (errors, warnings []string) {
	meta, body, err := parseFrontmatter(data)
	if err != nil {
		errors = append(errors, fmt.Sprintf("frontmatter: %v", err))
		return
	}
	_ = body

	if meta.Name == "" {
		errors = append(errors, "missing 'name'")
	} else if meta.Name != filename {
		errors = append(errors, fmt.Sprintf("'name' (%q) does not match filename (%q)", meta.Name, filename))
	}

	if meta.Description == "" {
		errors = append(errors, "missing 'description'")
	}

	if len(meta.Triggers) == 0 {
		errors = append(errors, "missing 'triggers'")
	} else {
		for i, t := range meta.Triggers {
			if !strings.HasPrefix(t, "/") {
				errors = append(errors, fmt.Sprintf("triggers[%d] (%q) must start with '/'", i, t))
			}
		}
	}

	if meta.Source == nil {
		warnings = append(warnings, "missing 'source' (set to empty string if hand-written)")
	}

	return
}

func parseFrontmatter(data []byte) (*skillMeta, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, data, fmt.Errorf("frontmatter must start with '---'")
	}

	end := bytes.Index(data[4:], []byte("\n---"))
	if end == -1 {
		return nil, data, fmt.Errorf("frontmatter not closed with '---'")
	}

	fm := data[4 : 4+end]
	var meta skillMeta
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return nil, data, fmt.Errorf("invalid YAML: %v", err)
	}

	body := data[4+end+4:]
	return &meta, body, nil
}

func init() {
	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsAddCmd)
	skillsCmd.AddCommand(skillsValidateCmd)
	skillsValidateCmd.Flags().BoolVar(&skillsValidateAll, "all", false, "Validate all skills")
	rootCmd.AddCommand(skillsCmd)
}
