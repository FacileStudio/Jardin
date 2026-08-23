package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A sync that stops and stays stopped is the state the bulk-delete guard can
// create, and doctor is where a human looks. The check reported the age and
// passed at any value, so a machine unsynced for a week read as healthy.
func TestLastSyncAgeFailsOnceItIsStale(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".sync-base.json")
	if err := os.WriteFile(base, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	written := time.Now()
	if err := os.Chtimes(base, written, written); err != nil {
		t.Fatal(err)
	}

	if _, ok := lastSyncAge(dir, written.Add(time.Hour)); !ok {
		t.Fatal("an hour old is a working machine, not a failure")
	}
	if _, ok := lastSyncAge(dir, written.Add(syncStaleAfter+time.Minute)); ok {
		t.Fatal("past the threshold the check must fail, not just report the number")
	}
	if _, ok := lastSyncAge(t.TempDir(), written); ok {
		t.Fatal("a machine that never synced is not healthy")
	}
}
