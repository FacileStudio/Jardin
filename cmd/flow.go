package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/FacileStudio/Mycelium/internal/flow"
	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/spf13/cobra"
)

var flowJSON bool
var flowRunsLimit int
var flowTrustYes bool
var flowQueryStatus string
var flowQuerySince string
var flowQueryFlow string

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Run recorded shell procedures and keep a record of every execution",
}

var flowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List flows with their step count and trust state",
	RunE: func(cmd *cobra.Command, args []string) error {
		flows, listErr := flow.List()
		if flowJSON {
			return printJSON(flowListRows(flows))
		}
		if len(flows) == 0 && listErr == nil {
			fmt.Println("No flows.")
			return nil
		}
		width := flowNameWidth(flows)
		for _, f := range flows {
			fmt.Println(flowRecapLine(f, width))
		}
		return listErr
	},
}

var flowAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Scaffold a new flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := flow.Scaffold(args[0])
		if err != nil {
			return err
		}
		ui.Success("Flow %q created at %s", args[0], path)
		ui.Hint("Edit it, then have it reviewed and pinned: mycelium flow trust %s", args[0])
		return nil
	},
}

var flowUntrustCmd = &cobra.Command{
	Use:   "untrust <name>",
	Short: "Drop a flow's pin so it must be reviewed again before it runs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			removed, err := flow.Prune()
			if err != nil {
				return err
			}
			ui.Success("Pruned %d pin(s) with no flow file.", removed)
			return nil
		}
		if err := flow.Untrust(args[0]); err != nil {
			return err
		}
		ui.Success("Dropped the pin for %q. It will refuse to run until trusted again.", args[0])
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
		ui.Step("Running %s (%s)", f.Name, stepCount(len(f.Steps)))
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

var flowTrustModelCmd = &cobra.Command{
	Use:   "trust-model <type>",
	Short: "Pin a model extension so typed steps may run it on this machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := flow.ModelPath(args[0])
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m := &flow.Model{Type: args[0], Path: path, Checksum: flow.Checksum(data)}
		if err := confirmModel(m, data); err != nil {
			return err
		}
		if err := flow.TrustModel(m); err != nil {
			return err
		}
		ui.Success("Trusted model %q. Typed steps may now run it here.", args[0])
		return nil
	},
}

var flowQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Search every flow's history at once",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		since, err := sessions.ParseSince(flowQuerySince, time.Now().UTC())
		if err != nil {
			return err
		}
		runs, err := flow.Query(flow.QueryOptions{
			Flow: flowQueryFlow, Status: flowQueryStatus, Since: since, Limit: flowRunsLimit,
		})
		if err != nil {
			return err
		}
		if flowJSON {
			return printJSON(flowQueryRows(runs))
		}
		if len(runs) == 0 {
			ui.Hint("No runs matched.")
			return nil
		}
		printQuery(runs)
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
		if pinned == f.Checksum {
			ui.Success("%q is already pinned at this exact content.", f.Name)
			return nil
		}
		if pinned == "" {
			ui.Step("First pin of %q on this machine. Read it before accepting it.", f.Name)
		} else {
			ui.Step("%q changed since it was approved here. Read what it does now.", f.Name)
			ui.Hint("approved %s", pinned)
		}
		if err := confirmTrust(f); err != nil {
			return err
		}
		if err := flow.Trust(f); err != nil {
			return err
		}
		ui.Success("Trusted %q. %s may now run here.", f.Name, stepCount(len(f.Steps)))
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
	flowCmd.AddCommand(flowAddCmd)
	flowCmd.AddCommand(flowRunCmd)
	flowCmd.AddCommand(flowRunsCmd)
	flowCmd.AddCommand(flowShowCmd)
	flowCmd.AddCommand(flowQueryCmd)
	flowCmd.AddCommand(flowTrustModelCmd)
	flowCmd.AddCommand(flowTrustCmd)
	flowCmd.AddCommand(flowUntrustCmd)
	flowQueryCmd.Flags().StringVar(&flowQueryStatus, "status", "", "Only runs with this status (ok, failed, timeout, unresolved)")
	flowQueryCmd.Flags().StringVar(&flowQuerySince, "since", "", "Only runs started within this window: 7d, 24h, 30m")
	flowQueryCmd.Flags().StringVar(&flowQueryFlow, "flow", "", "Only runs of this flow")
	for _, c := range []*cobra.Command{flowListCmd, flowRunsCmd, flowShowCmd, flowQueryCmd} {
		c.Flags().BoolVar(&flowJSON, "json", false, "Emit JSON")
	}
	flowTrustModelCmd.Flags().BoolVar(&flowTrustYes, "yes", false, "Pin without the interactive confirmation")
	flowTrustCmd.Flags().BoolVar(&flowTrustYes, "yes", false, "Pin without the interactive confirmation")
	flowRunsCmd.Flags().IntVar(&flowRunsLimit, "limit", 20, "How many runs to list")
	rootCmd.AddCommand(flowCmd)
}
