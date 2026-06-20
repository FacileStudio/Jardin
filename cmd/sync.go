package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/config"
	hsync "github.com/FacileStudio/Ruche/internal/sync"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Push and pull changes to sync server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		if cfg.SyncURL == "" {
			return fmt.Errorf("sync not configured — set sync_url in ~/.ruche/ruche.toml")
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		client := hsync.NewClient(cfg.SyncURL, cfg.SyncToken)

		pullPlan, err := client.Pull(cfg.ActiveCell, cellPath)
		if err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		for _, f := range pullPlan.Download {
			color.Cyan("  ↓ %s", f)
		}

		pushPlan, err := client.Push(cfg.ActiveCell, cellPath)
		if err != nil {
			return fmt.Errorf("push: %w", err)
		}
		for _, f := range pushPlan.Upload {
			color.Green("  ↑ %s", f)
		}

		total := len(pullPlan.Download) + len(pushPlan.Upload)
		if total == 0 {
			fmt.Println("Already in sync.")
		} else {
			fmt.Printf("Synced %d file(s).\n", total)
		}
		return nil
	},
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push local changes to sync server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		if cfg.SyncURL == "" {
			return fmt.Errorf("sync not configured — set sync_url in ~/.ruche/ruche.toml")
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		client := hsync.NewClient(cfg.SyncURL, cfg.SyncToken)
		plan, err := client.Push(cfg.ActiveCell, cellPath)
		if err != nil {
			return err
		}
		for _, f := range plan.Upload {
			color.Green("  ↑ %s", f)
		}
		if len(plan.Upload) == 0 {
			fmt.Println("Nothing to push.")
		}
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull changes from sync server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadRucheConfig()
		if err != nil {
			return err
		}
		if cfg.SyncURL == "" {
			return fmt.Errorf("sync not configured — set sync_url in ~/.ruche/ruche.toml")
		}
		cellPath, err := cfg.ActiveCellPath()
		if err != nil {
			return err
		}

		client := hsync.NewClient(cfg.SyncURL, cfg.SyncToken)
		plan, err := client.Pull(cfg.ActiveCell, cellPath)
		if err != nil {
			return err
		}
		for _, f := range plan.Download {
			color.Cyan("  ↓ %s", f)
		}
		if len(plan.Download) == 0 {
			fmt.Println("Nothing to pull.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
}
