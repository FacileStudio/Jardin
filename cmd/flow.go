package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/flow"
	"github.com/FacileStudio/Mycelium/internal/ui"
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
		for _, f := range flows {
			fmt.Printf("  %-28s %3d steps  %s\n", f.Name, len(f.Steps), trustState(f))
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

type flowListRow struct {
	Name     string `json:"name"`
	Steps    int    `json:"steps"`
	Checksum string `json:"checksum"`
	Trust    string `json:"trust"`
}

type flowRunRow struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
}

type flowRunJSON struct {
	ID string `json:"id"`
	*flow.Run
}

func flowListRows(flows []*flow.Flow) []flowListRow {
	rows := make([]flowListRow, 0, len(flows))
	for _, f := range flows {
		rows = append(rows, flowListRow{Name: f.Name, Steps: len(f.Steps), Checksum: f.Checksum, Trust: trustState(f)})
	}
	return rows
}

func flowRunRows(runs []*flow.Run) []flowRunRow {
	rows := make([]flowRunRow, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, flowRunRow{ID: r.ID, Status: r.Status, StartedAt: r.StartedAt,
			DurationMS: r.Duration().Milliseconds()})
	}
	return rows
}

func trustState(f *flow.Flow) string {
	pinned, err := flow.TrustedChecksum(f.Name)
	switch {
	case err != nil:
		return "unknown"
	case pinned == "":
		return "not pinned"
	case pinned == f.Checksum:
		return "trusted"
	default:
		return "CHANGED"
	}
}

func refuseUntrusted(f *flow.Flow) error {
	pinned, err := flow.TrustedChecksum(f.Name)
	if err != nil {
		return err
	}
	ui.Error("Refusing to run %q: it is not trusted on this machine.", f.Name)
	if pinned == "" {
		ui.Hint("Nothing is pinned here, so this machine has never approved this flow.")
		ui.Hint("current  %s", f.Checksum)
	} else {
		ui.Hint("The flow changed since it was approved on this machine.")
		ui.Hint("approved %s", pinned)
		ui.Hint("current  %s", f.Checksum)
		ui.Hint("It may have been changed on another machine or by the server.")
	}
	ui.Hint("Read it, then accept it with: mycelium flow trust %s", f.Name)
	return fmt.Errorf("flow %q is not trusted on this machine", f.Name)
}

func resolveRun(args []string) (*flow.Run, error) {
	if len(args) == 2 {
		return flow.LoadRun(args[0], args[1])
	}
	runs, err := flow.ListRuns(args[0], flowRunsLimit)
	if err != nil {
		return nil, err
	}
	var latest *flow.Run
	for _, r := range runs {
		if latest == nil || r.StartedAt.After(latest.StartedAt) {
			latest = r
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("flow %q has no runs yet", args[0])
	}
	return latest, nil
}

func reportRun(r *flow.Run, path string) error {
	summary := fmt.Sprintf("%d steps in %s", len(r.Steps), r.Duration().Round(time.Millisecond))
	if r.Status == flow.StatusOK {
		ui.Success("%s: %s", r.Flow, summary)
		ui.Hint("%s", path)
		return nil
	}
	ui.Error("%s: %s after %s", r.Flow, r.Status, summary)
	ui.Hint("%s", path)
	return fmt.Errorf("flow %q %s", r.Flow, r.Status)
}

func printRun(r *flow.Run) {
	ui.Step("%s  %s", r.Flow, r.ID)
	ui.Hint("machine %s   dir %s", r.Machine, r.WorkDir)
	ui.Hint("status %s   %s   %s", r.Status, r.StartedAt.Format(time.RFC3339), r.Duration().Round(time.Millisecond))
	ui.Hint("checksum %s", r.FlowChecksum)
	for _, s := range r.Steps {
		head := fmt.Sprintf("%s (exit %d, %dms)", s.Name, s.ExitCode, s.DurationMS)
		if s.ExitCode == 0 && !s.TimedOut {
			ui.Success("%s", head)
		} else {
			ui.Error("%s", head)
		}
		for _, stream := range []string{s.Stdout, s.Stderr} {
			if strings.TrimSpace(stream) != "" {
				fmt.Println(strings.TrimRight(stream, "\n"))
			}
		}
		if s.Truncated {
			ui.Hint("output truncated")
		}
	}
}

func machineName() string {
	if cfg, err := config.LoadMyceliumConfig(); err == nil && strings.TrimSpace(cfg.Machine) != "" {
		return strings.TrimSpace(cfg.Machine)
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
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
