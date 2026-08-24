package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/consolidate"
)

// consolidateHealth reports the state of the daemon's consolidation stage
// from its saved last-run file: whether it is enabled, when it last ran and
// what that run wrote. A machine with no events directory has nothing to
// consolidate and is not a failure; one with events but no runs is, because
// the daemon is either older than the stage or refusing to do its job.
func consolidateHealth(cfg *config.MyceliumConfig, dataDir string, now time.Time) (string, bool) {
	if !cfg.ConsolidateEnabled() {
		return "disabled", true
	}
	state, err := consolidate.LoadState(dataDir)
	if err != nil {
		return err.Error(), false
	}
	if _, err := os.Stat(filepath.Join(dataDir, "events")); os.IsNotExist(err) {
		return "no events on this machine", true
	}
	if state == nil || state.LastRun.IsZero() {
		return "enabled but never run — waiting for the next daemon tick", false
	}
	ago := now.Sub(state.LastRun).Truncate(time.Second)
	res := state.Result
	if res == nil {
		return fmt.Sprintf("last run %s ago, no counts recorded", ago), true
	}
	msg := fmt.Sprintf("last run %s ago: %d created, %d superseded, %d noop, %d dropped",
		ago, res.Created, res.Superseded, res.Noop, res.Dropped)
	if state.Error != "" {
		return msg + " — last run failed: " + state.Error, false
	}
	return msg, true
}
