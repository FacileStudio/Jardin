package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/daemon"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the background sync service",
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the background sync service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Install(); err != nil {
			return err
		}
		color.Green("Background sync enabled (every %ds).", daemon.IntervalSeconds)
		fmt.Println("Disable with: ruche daemon uninstall")
		return nil
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the background sync service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Uninstall(); err != nil {
			return err
		}
		color.Yellow("Background sync disabled.")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether background sync is installed",
	RunE: func(cmd *cobra.Command, args []string) error {
		if daemon.Installed() {
			color.Green("Background sync: installed")
		} else {
			fmt.Println("Background sync: not installed")
		}
		return nil
	},
}

var daemonRunCmd = &cobra.Command{
	Use:    "run",
	Short:  "Run one sync + install tick (used by the service)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run()
	},
}

func init() {
	daemonCmd.AddCommand(daemonInstallCmd, daemonUninstallCmd, daemonStatusCmd, daemonRunCmd)
	rootCmd.AddCommand(daemonCmd)
}
