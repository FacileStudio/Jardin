package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/FacileStudio/Ruche/internal/cell"
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/spf13/cobra"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Manage rules",
}

var rulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List rule files",
	RunE: func(cmd *cobra.Command, args []string) error {
		rules, err := cell.ListRules()
		if err != nil {
			return err
		}
		if len(rules) == 0 {
			fmt.Println("No rules.")
			return nil
		}
		for _, r := range rules {
			fmt.Println(r)
		}
		return nil
	},
}

var rulesEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Open a rule in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := filepath.Join(config.RulesDir(), args[0]+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.MkdirAll(config.RulesDir(), 0755)
			os.WriteFile(path, []byte(fmt.Sprintf("# %s\n", args[0])), 0644)
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	rulesCmd.AddCommand(rulesListCmd)
	rulesCmd.AddCommand(rulesEditCmd)
	rootCmd.AddCommand(rulesCmd)
}
