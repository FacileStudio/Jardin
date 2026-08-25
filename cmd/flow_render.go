package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/flow"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"golang.org/x/term"
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
	line := fmt.Sprintf("  %-*s  %-8s %-10s", width, f.Name, stepCount(len(f.Steps)), flow.TrustState(f))
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
			Name: f.Name, Description: f.Description, Steps: len(f.Steps), Checksum: f.Checksum, Trust: flow.TrustState(f),
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

type flowQueryRow struct {
	Flow       string    `json:"flow"`
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Failed     []string  `json:"failed_steps,omitempty"`
}

func flowQueryRows(runs []*flow.Run) []flowQueryRow {
	rows := make([]flowQueryRow, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, flowQueryRow{
			Flow: r.Flow, ID: r.ID, Status: r.Status, StartedAt: r.StartedAt,
			DurationMS: r.Duration().Milliseconds(), Failed: failedSteps(r),
		})
	}
	return rows
}

// failedSteps names the steps that actually broke, skipping the ones that only
// went down with them — a list of casualties buries the cause.
func failedSteps(r *flow.Run) []string {
	var names []string
	for _, s := range r.Steps {
		if s.Skipped || (s.ExitCode == 0 && !s.TimedOut) {
			continue
		}
		names = append(names, s.Name)
	}
	return names
}

func printQuery(runs []*flow.Run) {
	width := 0
	for _, r := range runs {
		if len(r.Flow) > width {
			width = len(r.Flow)
		}
	}
	for _, r := range runs {
		line := fmt.Sprintf("  %-*s  %-10s %-26s %10s", width, r.Flow, r.Status,
			r.StartedAt.Format(time.RFC3339), r.Duration().Round(time.Millisecond))
		if failed := failedSteps(r); len(failed) > 0 {
			line += "  " + ui.Dim("at "+strings.Join(failed, ", "))
		}
		fmt.Println(line)
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

func confirmTrust(f *flow.Flow) error {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s\n%s\n\n", ui.Dim("--- "+f.Path), strings.TrimRight(string(data), "\n"))
	ui.Hint("current  %s", f.Checksum)
	if flowTrustYes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("refusing to pin %q unreviewed: rerun with --yes once you have read it", f.Name)
	}
	fmt.Printf("Pin %s so it may run here? [y/N] ", f.Name)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return err
	}
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("flow %q was not pinned", f.Name)
	}
	return nil
}

// confirmModel shows the code before it is approved. A model is executed, not
// read, so the same rule as a flow applies: nothing runs on this machine until
// a person has looked at it here — and that means every file it imports, not
// just the entry, since a helper runs exactly as much as the entry does.
func confirmModel(m *flow.Model) error {
	for _, src := range m.Sources {
		fmt.Printf("\n%s\n%s\n", ui.Dim("--- "+src.Path), strings.TrimRight(string(src.Data), "\n"))
	}
	fmt.Println()
	if len(m.Sources) > 1 {
		ui.Hint("%d files: the entry and everything it imports", len(m.Sources))
	}
	ui.Hint("current  %s", m.Checksum)
	if flowTrustYes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("refusing to pin %q unreviewed: rerun with --yes once you have read it", m.Type)
	}
	fmt.Printf("Pin %s so typed steps may run it here? [y/N] ", m.Type)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return err
	}
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("model %q was not pinned", m.Type)
	}
	return nil
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
		if s.NotStarted {
			head = s.Name + " (did not start)"
		}
		if s.ExitCode == 0 && !s.TimedOut {
			ui.Success("%s", head)
		} else {
			ui.Error("%s", head)
		}
		printResolved(s)
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

// printResolved lists the values a step received from earlier steps, so a run
// can be read back without guessing what "$VERSION" held at the time.
func printResolved(s flow.StepResult) {
	names := make([]string, 0, len(s.Resolved))
	for name := range s.Resolved {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ui.Hint("%s=%s", name, s.Resolved[name])
	}
}
