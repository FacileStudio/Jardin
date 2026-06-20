package cmd

import (
	"github.com/FacileStudio/Ruche/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <cell>",
	Short: "Switch active cell",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}

		ref := cfg.FindCell(name)
		if ref == nil {
			return cmd.Help()
		}

		cfg.ActiveCell = name
		if err := config.SaveRucheConfig(cfg); err != nil {
			return err
		}

		color.Green("Active cell: %s", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
