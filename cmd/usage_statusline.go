package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/usage"
)

// runStatusLine is on the critical path of the user's prompt: it renders on
// nearly every keystroke, so it never fails the process and never prints a
// stack trace. A broken payload still yields a line.
func runStatusLine() {
	snapshot, parseErr := usage.ParseStatusLine(os.Stdin)
	if parseErr == nil {
		if machine := usageMachine(); machine != "" {
			usage.Record(config.DataDir(), machine, snapshot)
		}
	}
	fmt.Println(statusLineText(snapshot))
}

func statusLineText(s usage.Snapshot) string {
	parts := []string{"Mycelium"}
	for _, w := range s.Windows {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", usage.Short(w.Key), w.UsedPercentage))
	}
	if s.Model != "" {
		parts = append(parts, s.Model)
	}
	return strings.Join(parts, " · ")
}
