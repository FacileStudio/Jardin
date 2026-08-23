package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/journal"
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

// recordHistory files one operation in the local history, so a page this sync
// is about to overwrite or delete can still be recovered afterwards.
//
// Every failure is a warning and never an error. A corrupt repository, a held
// lock or a full disk must still leave the sync itself successful: memory that
// syncs without a history is what mycelium shipped for its whole life so far, and
// memory that refuses to sync is a machine cut off from its own wiki.
func recordHistory(dataDir, message string) {
	if err := journal.Commit(dataDir, message); err != nil {
		ui.Warn("history not recorded: %v", err)
	}
}

// recordLocalEdits commits anything written since the last sync before the
// reconcile touches it. Without it there is a window with no history at all: a
// page an agent wrote an hour ago and a pull then deletes was never in a commit,
// so nothing can give it back. Almost every run finds nothing and writes
// nothing.
//
// A failure here is silent, unlike the one after the sync. Both calls fail for
// the same reason and would print the same line, and a warning that always
// arrives twice is one people stop reading.
func recordLocalEdits(dataDir string) {
	_ = journal.Commit(dataDir, "local: written since the last sync")
}

// syncMessage describes a reconcile in the terms a human scanning the history
// wants: how many pages moved and in which direction. It is derived from the
// result and nothing else, because sync takes no arguments and an agent is
// never asked to narrate what it just did.
func syncMessage(res *hsync.Result) string {
	counts := []struct {
		n    int
		what string
	}{
		{len(res.Downloaded), "pulled %d"},
		{len(res.Uploaded), "pushed %d"},
		{len(res.DeletedLocal), "removed %d here"},
		{len(res.DeletedRemote), "removed %d on the server"},
		{len(res.Conflicts), "%d conflicted"},
	}
	var parts []string
	for _, c := range counts {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf(c.what, c.n))
		}
	}
	if len(parts) == 0 {
		return "sync: no changes"
	}
	return "sync: " + strings.Join(parts, ", ")
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
		recordLocalEdits(dataDir)
		res, err := client.Sync(dataDir)
		if err != nil {
			var refusal *hsync.BulkDeleteError
			if errors.As(err, &refusal) {
				reportBulkDelete(cmd, refusal)
			}
			return err
		}
		recordHistory(dataDir, syncMessage(res))
		printResult(res)
		if res.Total() == 0 {
			fmt.Println("Already in sync.")
		} else {
			fmt.Printf("Synced %d change(s).\n", res.Total())
		}
		warnFrenchPages(dataDir, res)
		if len(res.Conflicts) > 0 {
			color.Yellow("Resolve conflicts by editing the page, then delete its copy under .conflicts/ and sync again.")
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
		recordLocalEdits(dataDir)
		res, err := client.Pull(dataDir)
		if err != nil {
			return err
		}
		recordHistory(dataDir, syncMessage(res))
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
