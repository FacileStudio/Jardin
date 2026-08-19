package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/FacileStudio/Mycelium/internal/usage"
	"github.com/fatih/color"
)

// printSnapshot renders derived state, never the raw stored number on its own:
// a percentage shown as current when its window has already reset is the worst
// thing this command could do.
func printSnapshot(s usage.SnapshotView, self bool) {
	name := s.Machine
	if self {
		name += " (this machine)"
	}
	color.New(color.Bold).Print(name)
	age := "updated " + humanAgo(time.Duration(s.AgeSeconds)*time.Second)
	details := []string{s.Source, age}
	if s.Model != "" {
		details = append([]string{s.Model}, details...)
	}
	fmt.Printf("  %s", color.New(color.Faint).Sprint(strings.Join(details, " · ")))
	if s.Stale {
		color.New(color.FgYellow).Print("  stale — nobody has reported since")
	}
	fmt.Println()
	if len(s.Windows) == 0 {
		ui.Hint("no windows reported")
		return
	}
	width := 0
	for _, w := range s.Windows {
		if len(w.Label) > width {
			width = len(w.Label)
		}
	}
	for _, w := range s.Windows {
		fmt.Printf("  %-*s  ", width, w.Label)
		if w.Expired {
			fmt.Printf("%s %s  %s\n",
				color.New(color.Faint).Sprint(strings.Repeat("░", 20)),
				color.New(color.Faint).Sprintf("%5.1f%%", w.UsedPercentage),
				color.New(color.Faint).Sprintf("as of %s, window has since reset",
					humanAgo(time.Duration(s.AgeSeconds)*time.Second)))
			continue
		}
		fmt.Printf("%s %s", bar(w.UsedPercentage, 20), percentColor(w.UsedPercentage).Sprintf("%5.1f%%", w.UsedPercentage))
		if w.ResetsInSeconds != nil {
			fmt.Printf("  %s", color.New(color.Faint).Sprintf("resets in %s", humanUntil(time.Duration(*w.ResetsInSeconds)*time.Second)))
		}
		fmt.Println()
	}
}

func percentColor(pct float64) *color.Color {
	switch {
	case pct >= 90:
		return color.New(color.FgRed)
	case pct >= 70:
		return color.New(color.FgYellow)
	default:
		return color.New(color.FgGreen)
	}
}

func bar(pct float64, width int) string {
	filled := int(pct/100*float64(width) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return percentColor(pct).Sprint(strings.Repeat("█", filled)) +
		color.New(color.Faint).Sprint(strings.Repeat("░", width-filled))
}

func humanUntil(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func humanAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	return humanUntil(d) + " ago"
}
