package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/memory"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var memoryRatifyAll bool

var memoryRatifyCmd = &cobra.Command{
	Use:   "ratify [page...]",
	Short: "Accept a normative page as authoritative on this machine",
	Long: "Accept a normative page as authoritative on this machine.\n\n" +
		"A page carrying `type: standard` says what a repository must do, so a change to it " +
		"should land deliberately rather than appear everywhere five minutes later. " +
		"Ratifying pins the page at the content you just read. If it changes afterwards, " +
		"doctor fails and every search result from it is marked, until you read the new " +
		"version and ratify again.\n\n" +
		"Pins never leave the machine that made them. Ratifying on one machine says nothing " +
		"about any other, which is the point: a wrong edit made and accepted in one place is " +
		"still flagged everywhere else.\n\n" +
		"Run it with no arguments to see where every normative page stands here.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := config.DataDir()
		if len(args) == 0 && !memoryRatifyAll {
			return listNormativePages(dataDir)
		}
		pages := args
		if memoryRatifyAll {
			var err error
			pages, err = unratifiedOrChanged(dataDir)
			if err != nil {
				return err
			}
			if len(pages) == 0 {
				ui.Success("Every normative page is already ratified here.")
				return nil
			}
		}
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}
		if err := memory.Ratify(dataDir, cfg.Machine, time.Now(), pages); err != nil {
			return err
		}
		for _, page := range pages {
			ui.Success("Ratified %s at its current content.", page)
		}
		return nil
	},
}

var memoryForgetCmd = &cobra.Command{
	Use:   "forget <page...>",
	Short: "Drop the ratification of a page that has been deleted",
	Long: "Drop the ratification of a page that has been deleted.\n\n" +
		"A pin outlives the page it names, so a deleted standard reports MISSING until " +
		"someone confirms the deletion was meant. That is the point of the report; this is " +
		"how you close it.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := memory.Forget(config.DataDir(), args); err != nil {
			return err
		}
		for _, page := range args {
			ui.Success("Forgot the ratification of %s.", page)
		}
		return nil
	},
}

func listNormativePages(dataDir string) error {
	pages, err := memory.NormativePages(dataDir)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		fmt.Println("No normative pages in this wiki.")
		return nil
	}
	width := 0
	for _, page := range pages {
		if len(page.Path) > width {
			width = len(page.Path)
		}
	}
	needsWork := false
	for _, page := range pages {
		fmt.Printf("  %-*s  %-12s %s\n", width, page.Path, page.Standing, ui.Dim(acceptedAs(page)))
		if page.Standing != memory.Ratified {
			needsWork = true
		}
	}
	if needsWork {
		ui.Hint("Read a page, then accept it here with: mycelium memory ratify <page>")
	}
	return nil
}

// acceptedAs names the version this machine accepted, so a CHANGED page reads
// as "it moved since you accepted that one" rather than only as "it moved".
func acceptedAs(page memory.NormativePage) string {
	if page.On == "" {
		return ""
	}
	if page.Machine == "" {
		return "accepted " + page.On
	}
	return fmt.Sprintf("accepted %s on %s", page.On, page.Machine)
}

func unratifiedOrChanged(dataDir string) ([]string, error) {
	pages, err := memory.NormativePages(dataDir)
	if err != nil {
		return nil, err
	}
	var pending []string
	for _, page := range pages {
		if page.Standing == memory.Unratified || page.Standing == memory.Changed {
			pending = append(pending, page.Path)
		}
	}
	return pending, nil
}

func init() {
	memoryRatifyCmd.Flags().BoolVar(&memoryRatifyAll, "all", false,
		"Ratify every normative page that is not ratified or has changed")
	memoryCmd.AddCommand(memoryRatifyCmd)
	memoryCmd.AddCommand(memoryForgetCmd)
}

// normativeState reports whether any page that claims authority over a
// repository has moved since this machine accepted it.
//
// Only CHANGED and MISSING fail. A page nobody has ratified here is reported
// and passes, for the same reason the history check passes on a machine that
// has never started one: every fresh install is in that state, and a check that
// fails on every fresh install teaches its reader to skip the line. The
// pinning literature's own failure mode is a warning that fires so often it
// gets clicked through, and this is where that would start.
func normativeState(dataDir string) (string, bool) {
	pages, err := memory.NormativePages(dataDir)
	if err != nil {
		return err.Error(), false
	}
	if len(pages) == 0 {
		return "none in this wiki", true
	}
	counts := map[memory.Standing]int{}
	var firstBad memory.NormativePage
	for _, page := range pages {
		counts[page.Standing]++
		if !page.OK() && firstBad.Path == "" {
			firstBad = page
		}
	}
	if firstBad.Path != "" {
		fix := "ratify"
		if firstBad.Standing == memory.Missing {
			fix = "forget"
		}
		return fmt.Sprintf("%s %s — review it, then 'mycelium memory %s %s'",
			firstBad.Path, strings.ToLower(string(firstBad.Standing)), fix, firstBad.Path), false
	}
	if counts[memory.Unratified] > 0 {
		return fmt.Sprintf("%d of %d ratified here — read the rest, then 'mycelium memory ratify --all'",
			counts[memory.Ratified], len(pages)), true
	}
	return fmt.Sprintf("%d ratified here", len(pages)), true
}
