package cmd

import (
	"errors"
	"fmt"

	"github.com/FacileStudio/Jardin/internal/selfupdate"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"upgrade"},
	Short:   "Update jardin to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Current version: %s\n", version)

		if updateCheckOnly {
			latest, available, err := selfupdate.CheckLatest(version)
			if err != nil {
				return err
			}
			if available {
				color.Yellow("Update available: %s -> %s", version, latest)
			} else {
				color.Green("jardin is up to date (%s).", latest)
			}
			return nil
		}

		latest, available, err := selfupdate.CheckLatest(version)
		if err != nil {
			return err
		}
		if !available {
			color.Green("jardin is already up to date (%s).", latest)
			return nil
		}

		newVersion, err := selfupdate.Apply(version)
		if err != nil {
			if errors.Is(err, selfupdate.ErrHomebrew) {
				color.Yellow("jardin is managed by Homebrew. Run: brew upgrade jardin")
				return nil
			}
			return err
		}
		color.Green("Updated jardin %s -> %s", version, newVersion)
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Report whether an update is available without installing")
	rootCmd.AddCommand(updateCmd)
}
