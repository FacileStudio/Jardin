package cmd

import (
	"fmt"

	"github.com/FacileStudio/Ruche/internal/config"
	hsync "github.com/FacileStudio/Ruche/internal/sync"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func syncClient() (*hsync.Client, string, error) {
	cfg, err := config.LoadRucheConfig()
	if err != nil {
		return nil, "", err
	}
	if cfg.URL == "" {
		return nil, "", fmt.Errorf("sync not configured — run 'ruche login <url>'")
	}
	return hsync.NewClient(cfg.URL, cfg.Token), config.DataDir(), nil
}

func printResult(res *hsync.Result) {
	for _, f := range res.Downloaded {
		color.Cyan("  ↓ %s", f)
	}
	for _, f := range res.Uploaded {
		color.Green("  ↑ %s", f)
	}
	for _, f := range res.DeletedLocal {
		color.Red("  ✗ %s (removed locally)", f)
	}
	for _, f := range res.DeletedRemote {
		color.Red("  ✗ %s (removed on server)", f)
	}
	for _, f := range res.Conflicts {
		color.Yellow("  ! %s", f)
	}
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile local and server changes (push + pull + deletes)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, dataDir, err := syncClient()
		if err != nil {
			return err
		}
		res, err := client.Sync(dataDir)
		if err != nil {
			return err
		}
		printResult(res)
		if res.Total() == 0 {
			fmt.Println("Already in sync.")
		} else {
			fmt.Printf("Synced %d change(s).\n", res.Total())
		}
		if len(res.Conflicts) > 0 {
			color.Yellow("Resolve conflicts by editing the file and deleting its .conflict backup, then sync again.")
		}
		return nil
	},
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Force local changes up, overwriting the server",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, dataDir, err := syncClient()
		if err != nil {
			return err
		}
		res, err := client.Push(dataDir)
		if err != nil {
			return err
		}
		printResult(res)
		if res.Total() == 0 {
			fmt.Println("Nothing to push.")
		}
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Force server changes down, overwriting local",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, dataDir, err := syncClient()
		if err != nil {
			return err
		}
		res, err := client.Pull(dataDir)
		if err != nil {
			return err
		}
		printResult(res)
		if res.Total() == 0 {
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
