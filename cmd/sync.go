package cmd

import (
	"fmt"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/memory"
	hsync "github.com/FacileStudio/Jardin/internal/sync"
	"github.com/FacileStudio/Jardin/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func syncClient() (*hsync.Client, string, error) {
	cfg, err := config.LoadJardinConfig()
	if err != nil {
		return nil, "", err
	}
	if cfg.ServerURL() == "" {
		return nil, "", fmt.Errorf("sync not configured — run 'jardin login <url>'")
	}
	client := hsync.NewClient(cfg.ServerURL(), cfg.AuthToken())
	client.Space = cfg.Space
	return client, config.DataDir(), nil
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

// warnFrenchPages reports French prose in the pages this sync touched. The wiki
// is English-only (rules/20-memory.md) and a French page is unreachable from the
// English query an agent writes, so drift is worth surfacing the moment it
// crosses the wire.
//
// It only ever warns. Sync pulls as well as pushes, so failing here would lock a
// machine out of fetching the very fix it needs, and the daemon runs this every
// 60 seconds with nobody watching. The blocking gate lives in the jardin-health
// flow, where a human ran it and can act on the answer.
func warnFrenchPages(dataDir string, res *hsync.Result) {
	touched := append(append([]string{}, res.Uploaded...), res.Downloaded...)
	findings := memory.ScanPaths(dataDir, touched)
	if len(findings) == 0 {
		return
	}
	ui.Warn("%d French line(s) in %s", len(findings), findings[0].Path)
	if len(findings) > 1 {
		ui.Hint("first at %s:%d — run: bun ~/.jardin/skills/scripts/wiki-english-check.ts",
			findings[0].Path, findings[0].Line)
	} else {
		ui.Hint("at %s:%d — run: bun ~/.jardin/skills/scripts/wiki-english-check.ts",
			findings[0].Path, findings[0].Line)
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
		warnFrenchPages(dataDir, res)
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
