package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/flow"
	"github.com/FacileStudio/Jardin/internal/ui"
)

func flowNameWidth(flows []*flow.Flow) int {
	width := 0
	for _, f := range flows {
		if len(f.Name) > width {
			width = len(f.Name)
		}
	}
	return width
}

func stepCount(n int) string {
	if n == 1 {
		return "1 step"
	}
	return fmt.Sprintf("%d steps", n)
}

func flowRecapLine(f *flow.Flow, width int) string {
	line := fmt.Sprintf("  %-*s  %-8s %-10s", width, f.Name, stepCount(len(f.Steps)), trustState(f))
	if f.Description != "" {
		return line + "  " + f.Description
	}
	return strings.TrimRight(line, " ")
}

type flowListRow struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       int    `json:"steps"`
	Checksum    string `json:"checksum"`
	Trust       string `json:"trust"`
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
		rows = append(rows, flowListRow{
			Name: f.Name, Description: f.Description, Steps: len(f.Steps), Checksum: f.Checksum, Trust: trustState(f),
		})
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
	ui.Hint("Read it, then accept it with: jardin flow trust %s", f.Name)
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
	summary := fmt.Sprintf("%s in %s", stepCount(len(r.Steps)), r.Duration().Round(time.Millisecond))
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
	if cfg, err := config.LoadJardinConfig(); err == nil && strings.TrimSpace(cfg.Machine) != "" {
		return strings.TrimSpace(cfg.Machine)
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}
