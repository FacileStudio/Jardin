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
	Use:     "artifact",
	Aliases: []string{"artifacts", "report", "reports"},
	Short:   "Manage rendered artifacts and reports that travel between your machines",
}

var reportAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Record a markdown or HTML artifact and open it",
	Long: "Record a markdown or HTML artifact and open it.\n\n" +
		"The document is copied into ~/.mycelium/artifacts/ and synced across machines. " +
		"Markdown files (.md) are formatted with YAML frontmatter, and HTML files (.html) " +
		"are preserved for standalone viewing.\n\n" +
		"The identifier comes from the title with a date prefix to prevent collisions:\n\n" +
		"  mycelium artifact add spec.md\n" +
		"  mycelium artifact add audit.html --expires 7d\n" +
		"  mycelium artifact add plan.md --title 'Migration plan' --expires never",
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
	Short: "List the artifacts on this machine, newest first",
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
			fmt.Println("No artifacts")
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
	Short: "Open an artifact in this machine's browser",
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
	Short: "Delete an artifact",
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
	Short: "Delete every artifact whose expiry has passed",
	RunE: func(cmd *cobra.Command, args []string) error {
		swept, err := reports.Sweep(config.DataDir(), time.Now())
		if err != nil {
			return err
		}
		if len(swept) == 0 {
			fmt.Println("Nothing to sweep")
			return nil
		}
		ui.Success("Swept %d expired artifact(s): %s", len(swept), strings.Join(swept, ", "))
		return nil
	},
}

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
	ui.Hint("embed images as data: URIs or ensure paths are valid")
}

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

