package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/daemon"
	"github.com/FacileStudio/Mycelium/internal/memory"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var (
	memoryAddTitle     string
	memoryAddSource    string
	memoryAddBody      string
	memoryAddBodyStdin bool
	memoryAddLog       string
)

var memoryAddCmd = &cobra.Command{
	Use:   "add <page>",
	Short: "File a finding, with the bookkeeping that goes with it",
	Long: "File a finding, with the bookkeeping that goes with it.\n\n" +
		"Recording something learned takes four edits: the finding on its page, that page's " +
		"`updated:` stamp, a pointer in index.md, and a line in log.md. Done by hand the one " +
		"that gets skipped is always the index, because it is the only edit that is not where " +
		"the writing happened, and a router nobody updates stops routing. This does all four " +
		"in one call, or none of them.\n\n" +
		"The page is given the way the wiki names it, with or without the extension:\n\n" +
		"  mycelium memory add tools/mycelium --title 'What broke' \\\n" +
		"    --source 'direct observation' --body 'What was learned.' \\\n" +
		"    --log 'what was wrong before and what still holds'\n\n" +
		"Prose long enough to be worth keeping is prose long enough to fight the shell over " +
		"quoting, so --body-stdin reads the body from stdin instead:\n\n" +
		"  mycelium memory add bugs/some-bug --title '...' --source '...' --log '...' \\\n" +
		"    --body-stdin <<'EOF'\n" +
		"  Several paragraphs, quotes and backticks included.\n" +
		"  EOF\n\n" +
		"The log description is yours to write and is never derived from the change: it " +
		"records what was wrong before and what still holds, and a diff contains neither.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := findingBody()
		if err != nil {
			return err
		}
		finding := memory.Finding{
			Page:   args[0],
			Title:  memoryAddTitle,
			Source: memoryAddSource,
			Body:   body,
			Log:    memoryAddLog,
		}
		res, err := memory.Add(config.DataDir(), finding, time.Now(), pushAfterWrite)
		if err != nil {
			return err
		}
		reportAdd(res)
		return nil
	},
}

// findingBody reads the finding's prose, from stdin when asked, following the
// same shape as 'mycelium login --token-stdin'. Passing both flags is refused
// rather than resolved: there is no reading of that command where one of the
// two was not a mistake.
func findingBody() (string, error) {
	if memoryAddBodyStdin && memoryAddBody != "" {
		return "", fmt.Errorf("pass --body or --body-stdin, not both")
	}
	if !memoryAddBodyStdin {
		return memoryAddBody, nil
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read the body: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// pushAfterWrite sends the finding to the other machines now rather than at the
// next daemon tick, so a second agent on a second machine can read it within
// seconds of it being written.
//
// It is best effort by design and its error never reaches the caller as a
// failure. The finding is already on disk before this runs, memory is
// local-first, and a wiki that refuses to record what was learned because a
// server is unreachable is a wiki nobody trusts offline.
func pushAfterWrite() error {
	client, dataDir, err := syncClient()
	if err != nil {
		return err
	}
	recordLocalEdits(dataDir)
	res, err := client.Sync(dataDir)
	if err != nil {
		return err
	}
	recordHistory(dataDir, syncMessage(res))
	return nil
}

// reportAdd says what landed. The sync is reported as a warning and not an
// error, because everything it would have carried is already written here and
// the daemon retries every minute.
func reportAdd(res memory.Result) {
	ui.Success("Filed in %s.", res.Page)
	if res.Indexed {
		ui.Step("Added an index pointer for %s.", res.Page)
	}
	if res.SyncErr != nil {
		ui.Warn("Written here but not synced (%v)", res.SyncErr)
		ui.Hint("the daemon retries every %ds, or run 'mycelium sync'", daemon.IntervalSeconds)
	}
}

func init() {
	memoryAddCmd.Flags().StringVar(&memoryAddTitle, "title", "", "The finding's heading")
	memoryAddCmd.Flags().StringVar(&memoryAddSource, "source", "",
		"Where the claim comes from: a URL, a file path, or 'direct observation'")
	memoryAddCmd.Flags().StringVar(&memoryAddBody, "body", "", "The finding itself")
	memoryAddCmd.Flags().BoolVar(&memoryAddBodyStdin, "body-stdin", false,
		"Read the body from stdin instead of --body")
	memoryAddCmd.Flags().StringVar(&memoryAddLog, "log", "",
		"The log.md line: what was wrong before and what still holds")
	memoryCmd.AddCommand(memoryAddCmd)
}
