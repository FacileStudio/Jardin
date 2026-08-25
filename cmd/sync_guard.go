package cmd

import (
	"errors"
	"os"

	hsync "github.com/FacileStudio/Mycelium/internal/sync"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// errNotInteractive ends a destructive command nobody is watching.
//
// It carries no advice at all, on purpose. On 2026-08-25 a reconcile deleted
// 102 files here because the guard that stopped it printed the flag that
// waived it, and an agent whose goal is a finished sync reads that sentence as
// the next step rather than as a decision. Whatever an error says here, an
// agent can act on, so the only safe escape is one this process cannot reach.
var errNotInteractive = errors.New("refused: no terminal is attached")

// forceNeedsTerminal and pullNeedsTerminal name the power being refused rather
// than the command that was typed. Both hand a machine the ability to empty its
// own wiki in one call, and the party entitled to spend that is the one who can
// look at what would go.
const (
	forceNeedsTerminal = "'--force' accepts deletions nothing can undo, so it only runs from a terminal"
	pullNeedsTerminal  = "'mycelium pull' overwrites local files with the server's, so it only runs from a terminal"
)

// interactiveTerminal reports whether a human is watching this command run.
//
// It asks the same question login asks before opening a browser, on the same
// stream: a piped stdout is a daemon tick, a CI job or an agent's tool call,
// never somebody who can weigh what is about to be destroyed.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// requireTerminal refuses a destructive escape hatch when nothing is attached
// to read its output.
//
// The answer arrives as a parameter rather than being asked for here so both
// sides of the decision stay testable: a test binary never owns a terminal, so
// a guard that called interactiveTerminal itself could only ever be exercised
// in the direction that refuses.
func requireTerminal(cmd *cobra.Command, interactive bool, refusal string) error {
	if interactive {
		return nil
	}
	cmd.SilenceErrors = true
	ui.Error("%s", refusal)
	ui.ErrorHint("Nothing was changed. A human has to run this where they can read what would go.")
	return errNotInteractive
}

// bulkDeleteListLimit caps how many paths a refusal prints. A refused sync can
// name hundreds of files and the daemon pipes this straight into its failure
// report, so the list is trimmed to something a human can actually read.
const bulkDeleteListLimit = 10

// reportBulkDelete explains a refused sync: how many files would go, which ones
// and in which direction, how much each side still holds, and that nothing has
// changed yet. Cobra would print the same error again after this, so it is
// silenced and this block is the whole message.
//
// The last line depends on who is reading. A human at a prompt is exactly who
// --force exists for, so their copy names it. A non-interactive caller cannot
// legitimately accept these deletions at all, and telling it the flag is what
// turned this guard into a speed bump twice, so its copy names the review a
// human owes the corpus instead.
func reportBulkDelete(cmd *cobra.Command, interactive bool, refusal *hsync.BulkDeleteError) {
	cmd.SilenceErrors = true
	ui.Error("%v", refusal)
	listDeletions("removed here", refusal.Local)
	listDeletions("removed on the server", refusal.Remote)
	ui.ErrorHint("%s.", refusal.Inventory())
	if interactive {
		ui.ErrorHint("Nothing was deleted. Run 'mycelium sync --force' to accept these removals.")
		return
	}
	ui.ErrorHint("Nothing was deleted. This session has no terminal, so it cannot accept these removals.")
	ui.ErrorHint("A human has to review them on a terminal: %d here, %d on the server.",
		len(refusal.Local), len(refusal.Remote))
	ui.ErrorHint("A near-empty server is what caused this on 2026-08-19 and again on 2026-08-25.")
	ui.ErrorHint("Confirm the count independently before accepting any of it:")
	ui.ErrorHint(`  curl -H "Authorization: Bearer $TOKEN" "$SERVER/api/sync/tree"`)
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
