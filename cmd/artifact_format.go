package cmd

import (
	"fmt"
	"math"
	"time"

	"github.com/FacileStudio/Mycelium/internal/artifacts"
	"github.com/FacileStudio/Mycelium/internal/ui"
)

// artifactIDWidth is the column width the identifiers need so the titles line up.
func artifactIDWidth(all []artifacts.Artifact) int {
	width := 0
	for _, art := range all {
		if len(art.ID) > width {
			width = len(art.ID)
		}
	}
	return width
}

// artifactLine renders one listing row: what it is, which machine wrote it, and
// how long it has left.
func artifactLine(art artifacts.Artifact, width int, now time.Time) string {
	meta := artifactAge(art, now)
	if art.Machine != "" {
		meta = art.Machine + ", " + meta
	}
	return fmt.Sprintf("%-*s  %s  %s", width, art.ID, art.Title, ui.Dim("("+meta+")"))
}

// artifactAge pairs how old an artifact is with how long it has left, which is what
// decides whether to open it or delete it.
func artifactAge(art artifacts.Artifact, now time.Time) string {
	age := "just now"
	if d := now.Sub(art.Created); d >= time.Minute {
		age = humanDuration(d) + " ago"
	}
	if art.Expires.IsZero() {
		return age + ", pinned"
	}
	return age + ", expires in " + humanDuration(art.Expires.Sub(now))
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
