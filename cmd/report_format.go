package cmd

import (
	"fmt"
	"math"
	"time"

	"github.com/FacileStudio/Mycelium/internal/reports"
	"github.com/FacileStudio/Mycelium/internal/ui"
)

// reportIDWidth is the column width the identifiers need so the titles line up.
func reportIDWidth(all []reports.Report) int {
	width := 0
	for _, rep := range all {
		if len(rep.ID) > width {
			width = len(rep.ID)
		}
	}
	return width
}

// reportLine renders one listing row: what it is, which machine wrote it, and
// how long it has left.
func reportLine(rep reports.Report, width int, now time.Time) string {
	meta := reportAge(rep, now)
	if rep.Machine != "" {
		meta = rep.Machine + ", " + meta
	}
	return fmt.Sprintf("%-*s  %s  %s", width, rep.ID, rep.Title, ui.Dim("("+meta+")"))
}

// reportAge pairs how old a report is with how long it has left, which is what
// decides whether to open it or delete it.
func reportAge(rep reports.Report, now time.Time) string {
	age := "just now"
	if d := now.Sub(rep.Created); d >= time.Minute {
		age = humanDuration(d) + " ago"
	}
	if rep.Expires.IsZero() {
		return age + ", pinned"
	}
	return age + ", expires in " + humanDuration(rep.Expires.Sub(now))
}

// humanDuration renders a span the way somebody scanning a list wants it: one
// unit, never "743h".
//
// It rounds to nearest rather than truncating, because an expiry is read back
// in the unit it was asked for: "--expires 7d" is a few seconds short of seven
// days by the time it is printed, and truncation answers "expires in 6d".
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "0m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(math.Round(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(math.Round(d.Hours())))
	default:
		return fmt.Sprintf("%dd", int(math.Round(d.Hours()/24)))
	}
}
