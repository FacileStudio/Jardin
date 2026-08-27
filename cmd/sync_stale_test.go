package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/daemon"
)

// installDaemonMarker fakes the file daemon.Installed() looks for, so a test
// can ask both questions without touching the live unit on this machine.
func installDaemonMarker(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mycelium-sync.timer"), []byte("[Timer]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plistDir, daemon.Label+".plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSyncBase plants a completed sync at the given age.
func writeSyncBase(t *testing.T, dataDir string, age time.Duration) time.Time {
	t.Helper()
	base := filepath.Join(dataDir, ".sync-base.json")
	if err := os.WriteFile(base, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(base, when, when); err != nil {
		t.Fatal(err)
	}
	return when
}

// The threshold has to follow the cadence that feeds it. A 24h constant against
// a 60s timer is a 1440x gap, which is how a sync broken for twenty hours on
// 2026-08-25 still drew a green tick from doctor.
func TestSyncStaleAfterFollowsTheDaemonCadence(t *testing.T) {
	tick := time.Duration(daemon.IntervalSeconds) * time.Second
	got := syncStaleAfter(true)
	if got != staleSyncTicks*tick {
		t.Fatalf("with the daemon installed the threshold must be %d ticks (%s), got %s",
			staleSyncTicks, staleSyncTicks*tick, got)
	}
	if got >= time.Hour {
		t.Fatalf("a %s threshold on a %s tick is a calendar, not a heartbeat", got, tick)
	}
	if got := syncStaleAfter(false); got != manualSyncStaleAfter {
		t.Fatalf("without the daemon a human syncs by hand: want %s, got %s", manualSyncStaleAfter, got)
	}
}

// The same age reads differently depending on who is meant to be syncing. This
// is the whole point of deriving the threshold rather than fixing it: eleven
// minutes is a stopped daemon and an unremarkable manual gap at once.
func TestLastSyncAgeSplitsOnWhoIsSyncing(t *testing.T) {
	dir := t.TempDir()
	written := writeSyncBase(t, dir, 0)
	age := syncStaleAfter(true) + time.Minute

	if _, ok := lastSyncAge(dir, written.Add(age), syncStaleAfter(true)); ok {
		t.Fatalf("%s without a tick from a 60s daemon is a failure", age)
	}
	if _, ok := lastSyncAge(dir, written.Add(age), syncStaleAfter(false)); !ok {
		t.Fatalf("%s is nothing on a machine that syncs by hand", age)
	}
}

// recap runs at the start of every agent session, so it has to be silent while
// sync works. A warning printed every time is one nobody reads by the third.
func TestSyncRecapIsSilentUntilSyncStops(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DATA_DIR", data)
	installDaemonMarker(t, home)

	writeSyncBase(t, data, time.Minute)
	if got := syncRecap(); got != "" {
		t.Fatalf("a minute-old sync is healthy, recap must say nothing, got %q", got)
	}

	writeSyncBase(t, data, syncStaleAfter(true)+time.Minute)
	got := syncRecap()
	if !strings.Contains(got, "stale") || !strings.Contains(got, "mycelium sync") {
		t.Fatalf("a stopped daemon must reach the session and name the fix, got %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("the recap line is one line, got %q", got)
	}
}

// A machine with no daemon does not get the short threshold shouted at it. The
// staleness line only means something where a service was supposed to sync.
func TestSyncRecapStaysQuietWithoutADaemon(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DATA_DIR", data)

	writeSyncBase(t, data, syncStaleAfter(true)+time.Minute)
	if got := syncRecap(); got != "" {
		t.Fatalf("no daemon installed, so this age is not a failure, got %q", got)
	}
}
