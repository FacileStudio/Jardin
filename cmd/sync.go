package cmd

import (
	"errors"
	"fmt"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/memory"
	hsync "github.com/FacileStudio/Mycelium/internal/sync"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func syncClient() (*hsync.Client, string, error) {
	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		return nil, "", err
	}
	if cfg.ServerURL() == "" {
		return nil, "", fmt.Errorf("sync not configured — run 'mycelium login <url>'")
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
// 60 seconds with nobody watching. The blocking gate lives in the mycelium-health
// flow, where a human ran it and can act on the answer.
func warnFrenchPages(dataDir string, res *hsync.Result) {
	touched := append(append([]string{}, res.Uploaded...), res.Downloaded...)
	findings := memory.ScanPaths(dataDir, touched)
	if len(findings) == 0 {
		return
	}
	ui.Warn("%d French line(s) in %s — the wiki is English-only", len(findings), findings[0].Path)
	ui.Hint("first at %s:%d — see 'mycelium doctor' for the whole corpus", findings[0].Path, findings[0].Line)
}

// bulkDeleteListLimit caps how many paths a refusal prints. A refused sync can
// name hundreds of files and the daemon pipes this straight into its failure
// report, so the list is trimmed to something a human can actually read.
const bulkDeleteListLimit = 10

// reportBulkDelete explains a refused sync: how many files would go, which ones
// and in which direction, that nothing has changed yet, and the one flag that
// accepts the removals. Cobra would print the same error again after this, so
// it is silenced and this block is the whole message.
func reportBulkDelete(cmd *cobra.Command, refusal *hsync.BulkDeleteError) {
	cmd.SilenceErrors = true
	ui.Error("%v", refusal)
	listDeletions("removed here", refusal.Local)
	listDeletions("removed on the server", refusal.Remote)
	ui.ErrorHint("Nothing was deleted. Run 'mycelium sync --force' to accept these removals.")
}

// listDeletions names the files one direction of a refusal would destroy. It
// stays on stderr with the error it belongs to, so redirecting one stream never
// separates the count from the paths that explain it.
func listDeletions(what string, paths []string) {
	if len(paths) == 0 {
		return
	}
	ui.ErrorHint("%s:", what)
	for _, p := range paths[:min(len(paths), bulkDeleteListLimit)] {
		ui.ErrorHint("  %s", p)
	}
	if extra := len(paths) - bulkDeleteListLimit; extra > 0 {
		ui.ErrorHint("  and %d more", extra)
	}
}

var syncCmd = &cobra.Command{
	Use:          "sync",
	Short:        "Reconcile local and server changes (push + pull + deletes)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, dataDir, err := syncClient()
		if err != nil {
			return err
		}
		client.AllowBulkDelete, _ = cmd.Flags().GetBool("force")
		res, err := client.Sync(dataDir)
		if err != nil {
			var refusal *hsync.BulkDeleteError
			if errors.As(err, &refusal) {
				reportBulkDelete(cmd, refusal)
			}
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
		warnFrenchPages(dataDir, res)
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
	syncCmd.Flags().Bool("force", false,
		fmt.Sprintf("Accept a sync that would delete more than %d files", hsync.MaxSilentDeletes))
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
}
