package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/FacileStudio/Jardin/internal/flow"
	"github.com/FacileStudio/Jardin/internal/ui"
	"github.com/spf13/cobra"
)

var flowJSON bool
var flowRunsLimit int

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Run recorded shell procedures and keep a record of every execution",
}

var flowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List flows with their step count and trust state",
	RunE: func(cmd *cobra.Command, args []string) error {
		flows, err := flow.List()
		if err != nil {
			return err
		}
		if flowJSON {
			return printJSON(flowListRows(flows))
		}
		if len(flows) == 0 {
			fmt.Println("No flows.")
			return nil
		}
		width := flowNameWidth(flows)
		for _, f := range flows {
			fmt.Println(flowRecapLine(f, width))
		}
		return nil
	},
}

var flowRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Execute a flow, stream its output and write the run artifact",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := flow.Load(args[0])
		if err != nil {
			return err
		}
		trusted, err := flow.IsTrusted(f)
		if err != nil {
			return err
		}
		if !trusted {
			return refuseUntrusted(f)
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		ui.Step("Running %s (%d steps)", f.Name, len(f.Steps))
		opts := flow.Options{WorkDir: dir, Machine: machineName(), Stream: os.Stdout}
		run := flow.Execute(cmd.Context(), f, opts)
		path, err := flow.SaveRun(run)
		if err != nil {
			return err
		}
		return reportRun(run, path)
	},
}

var flowRunsCmd = &cobra.Command{
	Use:   "runs <name>",
	Short: "List recent runs of a flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runs, err := flow.ListRuns(args[0], flowRunsLimit)
		if err != nil {
			return err
		}
		if flowJSON {
			return printJSON(flowRunRows(runs))
		}
		if len(runs) == 0 {
			fmt.Printf("No runs for %q.\n", args[0])
			return nil
		}
		for _, r := range runs {
			fmt.Printf("  %-26s %-8s %10s  %s\n", r.StartedAt.Format(time.RFC3339),
				r.Status, r.Duration().Round(time.Millisecond), ui.Dim(r.ID))
		}
		return nil
	},
}

var flowShowCmd = &cobra.Command{
	Use:   "show <name> [run]",
	Short: "Show one run in full, defaulting to the latest",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := resolveRun(args)
		if err != nil {
			return err
		}
		if flowJSON {
			return printJSON(flowRunJSON{ID: r.ID, Run: r})
		}
		printRun(r)
		return nil
	},
}

var flowTrustCmd = &cobra.Command{
	Use:   "trust <name>",
	Short: "Pin the current checksum of a flow so it may run on this machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := flow.Load(args[0])
		if err != nil {
			return err
		}
		pinned, err := flow.TrustedChecksum(f.Name)
		if err != nil {
			return err
		}
		if pinned == "" {
			ui.Step("First pin of %q on this machine.", f.Name)
		} else {
			ui.Step("%q was already pinned on this machine.", f.Name)
			ui.Hint("approved %s", pinned)
		}
		ui.Hint("current  %s", f.Checksum)
		if err := flow.Trust(f); err != nil {
			return err
		}
		ui.Success("Trusted %q. %d steps may now run here.", f.Name, len(f.Steps))
		return nil
	},
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	flowCmd.AddCommand(flowListCmd)
	flowCmd.AddCommand(flowRunCmd)
	flowCmd.AddCommand(flowRunsCmd)
	flowCmd.AddCommand(flowShowCmd)
	flowCmd.AddCommand(flowTrustCmd)
	for _, c := range []*cobra.Command{flowListCmd, flowRunsCmd, flowShowCmd} {
		c.Flags().BoolVar(&flowJSON, "json", false, "emit JSON")
	}
	flowRunsCmd.Flags().IntVar(&flowRunsLimit, "limit", 20, "how many runs to list")
	rootCmd.AddCommand(flowCmd)
}
