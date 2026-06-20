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
	Short: "Manage cell rules",
}

var rulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List rule files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		rules, err := cell.ListRules(cellPath)
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
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		path := filepath.Join(cellPath, "rules", args[0]+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
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
