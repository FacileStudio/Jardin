package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/artifacts"
	"github.com/FacileStudio/Mycelium/internal/browser"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var (
	artifactJSON      bool
	artifactTitle     string
	artifactExpires   string
	artifactNoOpen    bool
	artifactBodyStdin bool
)

var artifactCmd = &cobra.Command{
	Use:     "artifact",
	Aliases: []string{"artifacts", "report", "reports"},
	Short:   "Manage rendered artifacts and reports that travel between your machines",
}

var artifactAddCmd = &cobra.Command{
	Use:   "add [file]",
	Short: "Record a markdown or HTML artifact and open it",
	Long: "Record a markdown or HTML artifact and open it.\n\n" +
		"The document is copied into ~/.mycelium/artifacts/ and synced across machines. " +
		"Markdown files (.md) are formatted with YAML frontmatter, and HTML files (.html) " +
		"are preserved for standalone viewing.\n\n" +
		"The identifier comes from the title with a date prefix to prevent collisions:\n\n" +
		"  mycelium artifact add spec.md\n" +
		"  mycelium artifact add audit.html --expires 7d\n" +
		"  mycelium artifact add plan.md --title 'Migration plan' --expires never\n" +
		"  mycelium artifact add --title 'Quick note' --body-stdin < report.md",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		expires, pinned, err := parseExpiry(artifactExpires)
		if err != nil {
			return err
		}
		var art artifacts.Artifact
		fromStdin := artifactBodyStdin || (len(args) == 1 && args[0] == "-")
		if fromStdin {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			art, err = artifacts.AddContent(config.DataDir(), raw, "stdin.md", artifacts.Request{
				Title:   artifactTitle,
				Machine: config.MachineName(),
				Expires: expires,
				Pinned:  pinned,
			}, time.Now())
			if err != nil {
				return err
			}
		} else {
			if len(args) == 0 {
				return errors.New("specify a file to record or pass --body-stdin")
			}
			art, err = artifacts.Add(config.DataDir(), artifacts.Request{
				Source:  args[0],
				Title:   artifactTitle,
				Machine: config.MachineName(),
				Expires: expires,
				Pinned:  pinned,
			}, time.Now())
			if err != nil {
				return err
			}
			warnExternalRefs(args[0])
		}
		artifactRecorded(art)
		return nil
	},
}

var artifactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the artifacts on this machine, newest first",
	RunE: func(cmd *cobra.Command, args []string) error {
		artifacts.Sweep(config.DataDir(), time.Now())
		all, err := artifacts.List(config.DataDir())
		if err != nil {
			return err
		}
		if artifactJSON {
			return printJSON(all)
		}
		if len(all) == 0 {
			fmt.Println("No artifacts")
			return nil
		}
		width := artifactIDWidth(all)
		for _, art := range all {
			fmt.Println(artifactLine(art, width, time.Now()))
		}
		return nil
	},
}

var artifactOpenCmd = &cobra.Command{
	Use:   "open <id>",
	Short: "Open an artifact in this machine's browser",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		art, err := artifacts.Find(config.DataDir(), args[0])
		if err != nil {
			return err
		}
		target := artifactTarget(art)
		fmt.Println(target)
		showInBrowser(target)
		return nil
	},
}

var artifactRmCmd = &cobra.Command{
	Use:   "rm <id>",
	Short: "Delete an artifact",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := artifacts.Remove(config.DataDir(), args[0]); err != nil {
			return err
		}
		ui.Success("Deleted %s", artifacts.Slug(args[0]))
		return nil
	},
}

var artifactSweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Delete every artifact whose expiry has passed",
	RunE: func(cmd *cobra.Command, args []string) error {
		swept, err := artifacts.Sweep(config.DataDir(), time.Now())
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
	refs := artifacts.ExternalRefs(raw)
	if len(refs) == 0 {
		return
	}
	ui.Warn("%d reference(s) will not resolve from disk: %s", len(refs), strings.Join(refs, ", "))
	ui.Hint("embed images as data: URIs or ensure paths are valid")
}

func artifactTarget(art artifacts.Artifact) string {
	cfg, err := config.LoadMyceliumConfig()
	if err == nil && cfg != nil && cfg.ServerURL() != "" {
		return artifacts.URL(cfg.ServerURL(), art.ID)
	}
	return art.Path
}

func artifactRecorded(art artifacts.Artifact) {
	target := artifactTarget(art)
	ui.Success("Recorded %s", art.ID)
	ui.Hint("%s", target)
	if err := pushAfterWrite(); err != nil {
		ui.Warn("Recorded here but not synced (%v)", err)
	}
	if artifactNoOpen {
		return
	}
	showInBrowser(target)
}

// showInBrowser opens what was just printed, and says why it did not when this
// machine has no browser. The link is already on stdout by then, so nothing has
// failed: a headless box gets the link and one line explaining the silence,
// never an error and never an exit code.
func showInBrowser(target string) {
	if !browser.Available() {
		ui.Hint("no browser on this machine")
		return
	}
	if err := browser.Open(target); err != nil {
		ui.Warn("Could not open a browser (%v)", err)
	}
}

func init() {
	artifactAddCmd.Flags().StringVar(&artifactTitle, "title", "", "Override the document's own title")
	artifactAddCmd.Flags().StringVar(&artifactExpires, "expires", "",
		"How long to keep it: 7d, 12h, or never (default 30d)")
	artifactAddCmd.Flags().BoolVar(&artifactNoOpen, "no-open", false, "Record it without opening a browser")
	artifactAddCmd.Flags().BoolVar(&artifactBodyStdin, "body-stdin", false, "Read artifact body from stdin")
	artifactAddCmd.Flags().BoolVar(&artifactBodyStdin, "stdin", false, "Read artifact body from stdin")
	artifactListCmd.Flags().BoolVar(&artifactJSON, "json", false, "Print the listing as JSON")
	artifactCmd.AddCommand(artifactAddCmd, artifactListCmd, artifactOpenCmd, artifactRmCmd, artifactSweepCmd)
	rootCmd.AddCommand(artifactCmd)
}
