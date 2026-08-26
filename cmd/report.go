package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/reports"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var (
	reportJSON    bool
	reportTitle   string
	reportExpires string
	reportNoOpen  bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Manage rendered pages that travel between your machines",
}

var reportAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Record a self-contained HTML page and open it",
	Long: "Record a self-contained HTML page and open it.\n\n" +
		"The page is copied into ~/.mycelium/reports/ and synced, so one written on a headless " +
		"machine opens on the machine you are sitting at. It is never hosted: a file opened from " +
		"disk gets its own opaque origin, which is what keeps a generated page away from the " +
		"credentials the web UI holds.\n\n" +
		"A report is derived output, not memory. Findings go in the wiki as text with " +
		"'mycelium memory add'; a report is the picture of one, and it expires.\n\n" +
		"The identifier comes from the document's <title>, so recording the same page twice " +
		"replaces it rather than piling up copies:\n\n" +
		"  mycelium report add /tmp/drift.html\n" +
		"  mycelium report add /tmp/drift.html --expires 7d\n" +
		"  mycelium report add /tmp/drift.html --title 'Suite drift' --expires never\n\n" +
		"The page must carry everything it needs. A relative src or href cannot resolve from " +
		"disk, so 'add' names any it finds rather than letting the reader discover them.",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		expires, pinned, err := parseExpiry(reportExpires)
		if err != nil {
			return err
		}
		rep, err := reports.Add(config.DataDir(), reports.Request{
			Source:  args[0],
			Title:   reportTitle,
			Machine: config.MachineName(),
			Expires: expires,
			Pinned:  pinned,
		}, time.Now())
		if err != nil {
			return err
		}
		warnExternalRefs(args[0])
		reportRecorded(rep)
		return nil
	},
}

var reportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the reports on this machine, newest first",
	RunE: func(cmd *cobra.Command, args []string) error {
		reports.Sweep(config.DataDir(), time.Now())
		all, err := reports.List(config.DataDir())
		if err != nil {
			return err
		}
		if reportJSON {
			return printJSON(all)
		}
		if len(all) == 0 {
			fmt.Println("No reports")
			return nil
		}
		width := reportIDWidth(all)
		for _, rep := range all {
			fmt.Println(reportLine(rep, width, time.Now()))
		}
		return nil
	},
}

var reportOpenCmd = &cobra.Command{
	Use:   "open <id>",
	Short: "Open a report in this machine's browser",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rep, err := reports.Find(config.DataDir(), args[0])
		if err != nil {
			return err
		}
		if !reports.HasDisplay() {
			fmt.Println(rep.Path)
			return nil
		}
		return reports.Open(rep.Path)
	},
}

var reportRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete a report",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := reports.Remove(config.DataDir(), args[0]); err != nil {
			return err
		}
		ui.Success("Deleted %s", reports.Slug(args[0]))
		return nil
	},
}

var reportSweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Delete every report whose expiry has passed",
	RunE: func(cmd *cobra.Command, args []string) error {
		swept, err := reports.Sweep(config.DataDir(), time.Now())
		if err != nil {
			return err
		}
		if len(swept) == 0 {
			fmt.Println("Nothing to sweep")
			return nil
		}
		ui.Success("Swept %d expired report(s): %s", len(swept), strings.Join(swept, ", "))
		return nil
	},
}

// parseExpiry reads --expires. Days are spelled the way a human writes them,
// which time.ParseDuration refuses to, and "never" pins the report instead of
// dating it.
func parseExpiry(raw string) (time.Time, bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case value == "":
		return time.Time{}, false, nil
	case value == "never":
		return time.Time{}, true, nil
	case strings.HasSuffix(value, "d"):
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days < 0 {
			return time.Time{}, false, fmt.Errorf("invalid expiry %q, try 7d or 12h or never", raw)
		}
		return time.Now().AddDate(0, 0, days), false, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid expiry %q, try 7d or 12h or never", raw)
	}
	return time.Now().Add(d), false, nil
}

// warnExternalRefs names the assets the page will not find once it is opened
// from disk. It warns rather than refuses: a broken image is the author's call
// to make, and a page held hostage over a favicon helps nobody.
func warnExternalRefs(source string) {
	raw, err := os.ReadFile(source)
	if err != nil {
		return
	}
	refs := reports.ExternalRefs(raw)
	if len(refs) == 0 {
		return
	}
	ui.Warn("%d reference(s) will not resolve from disk: %s", len(refs), strings.Join(refs, ", "))
	ui.Hint("inline the CSS and JS, and use data: URIs for images")
}

// reportRecorded says what landed and where, and opens the page when there is
// somebody in front of this machine to see it.
func reportRecorded(rep reports.Report) {
	ui.Success("Recorded %s", rep.ID)
	ui.Hint("%s", rep.Path)
	if err := pushAfterWrite(); err != nil {
		ui.Warn("Recorded here but not synced (%v)", err)
	}
	if reportNoOpen || !reports.HasDisplay() {
		return
	}
	if err := reports.Open(rep.Path); err != nil {
		ui.Warn("Could not open a browser (%v)", err)
	}
}

func init() {
	reportAddCmd.Flags().StringVar(&reportTitle, "title", "", "Override the document's own title")
	reportAddCmd.Flags().StringVar(&reportExpires, "expires", "",
		"How long to keep it: 7d, 12h, or never (default 30d)")
	reportAddCmd.Flags().BoolVar(&reportNoOpen, "no-open", false, "Record it without opening a browser")
	reportListCmd.Flags().BoolVar(&reportJSON, "json", false, "Print the listing as JSON")
	reportCmd.AddCommand(reportAddCmd, reportListCmd, reportOpenCmd, reportRmCmd, reportSweepCmd)
	rootCmd.AddCommand(reportCmd)
}
