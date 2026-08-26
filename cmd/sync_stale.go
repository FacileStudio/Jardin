package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FacileStudio/Mycelium/internal/daemon"
)

// staleSyncTicks is how many daemon ticks may pass without a completed sync
// before doctor calls it a failure. A heartbeat threshold belongs to the
// cadence that produces it, never to a round human number: against a 60s timer
// the old 24h constant let sync fail for twenty hours on 2026-08-25 while
// doctor printed a green tick beside "19h43m36s ago".
//
// Thirty ticks, not ten. recap prints this line unbidden at the start of every
// session, and ten minutes is shorter than a lunch break, a train tunnel or a
// closed laptop lid — so on anything but an always-on machine the line would
// appear on most mornings, and a warning that common is one nobody reads by
// the third time. Half an hour still surfaces a stopped daemon within the hour
// and still clears itself one tick after the machine is back.
const staleSyncTicks = 30

// manualSyncStaleAfter applies when no background service is installed. Syncing
// is then something a human does when they think of it, so the gap between two
// of them says nothing about whether anything is broken, and a short threshold
// would fail on every machine that syncs by hand.
const manualSyncStaleAfter = 24 * time.Hour

// syncStaleAfter returns the age past which the last completed sync counts as a
// failure. daemonInstalled decides which question is being asked: whether a
// service that runs every minute has stopped, or whether a person has synced
// lately.
func syncStaleAfter(daemonInstalled bool) time.Duration {
	if !daemonInstalled {
		return manualSyncStaleAfter
	}
	return staleSyncTicks * time.Duration(daemon.IntervalSeconds) * time.Second
}

// lastSyncAge reports how long ago the base manifest was written and whether
// that is recent enough to call healthy. .sync-base.json records the last
// success, not the last attempt: a sync that fails never touches it, which is
// what makes the mtime a usable heartbeat. now and staleAfter are parameters so
// the threshold is testable without waiting for it, and so recap can ask doctor
// its own question instead of reimplementing it.
func lastSyncAge(dataDir string, now time.Time, staleAfter time.Duration) (string, bool) {
	info, err := os.Stat(filepath.Join(dataDir, ".sync-base.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "never synced", false
		}
		return err.Error(), false
	}
	ago := now.Sub(info.ModTime()).Truncate(time.Second)
	if ago > staleAfter {
		return fmt.Sprintf("%s ago, run 'mycelium sync' to see why", ago), false
	}
	return fmt.Sprintf("%s ago", ago), true
}
