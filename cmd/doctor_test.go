package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/consolidate"
	"github.com/FacileStudio/Mycelium/internal/memory"
)

func writeStandard(t *testing.T, dataDir, rel, body string) {
	t.Helper()
	path := filepath.Join(dataDir, "memory", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	page := "---\ntitle: A standard\ntype: standard\n---\n\n" + body
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The states that matter to a human reading doctor. Only a page that moved
// after this machine accepted it, or one that vanished, is a failure — an
// unratified page is what every fresh install looks like, and failing there
// would train the reader to skip the line.
func TestNormativeStateFailsOnlyOnChangedOrMissing(t *testing.T) {
	dir := t.TempDir()
	writeStandard(t, dir, "standards/cli.md", "rule one")

	msg, ok := normativeState(dir)
	if !ok || !strings.Contains(msg, "0 of 1 ratified") {
		t.Fatalf("unratified must pass and say so, got %q ok=%v", msg, ok)
	}

	if err := memory.Ratify(dir, "lucy", time.Now(), []string{"standards/cli.md"}); err != nil {
		t.Fatal(err)
	}
	if msg, ok := normativeState(dir); !ok || !strings.Contains(msg, "1 ratified here") {
		t.Fatalf("a ratified wiki must pass cleanly, got %q ok=%v", msg, ok)
	}

	writeStandard(t, dir, "standards/cli.md", "rule one, reversed")
	msg, ok = normativeState(dir)
	if ok || !strings.Contains(msg, "mycelium memory ratify standards/cli.md") {
		t.Fatalf("a changed standard must fail and name the fix, got %q ok=%v", msg, ok)
	}

	if err := os.Remove(filepath.Join(dir, "memory", "standards", "cli.md")); err != nil {
		t.Fatal(err)
	}
	msg, ok = normativeState(dir)
	if ok || !strings.Contains(msg, "mycelium memory forget standards/cli.md") {
		t.Fatalf("a deleted standard must fail and point at forget, got %q ok=%v", msg, ok)
	}
}

// A wiki with no standards, and a machine with no wiki at all, are both healthy.
func TestNormativeStatePassesWithNothingToGovern(t *testing.T) {
	if msg, ok := normativeState(t.TempDir()); !ok || msg != "none in this wiki" {
		t.Fatalf("got %q ok=%v", msg, ok)
	}
}

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

func TestConsolidateHealthStates(t *testing.T) {
	cfg := &config.MyceliumConfig{}
	disabled := *cfg
	off := false
	disabled.Consolidate.Enabled = &off
	if msg, ok := consolidateHealth(&disabled, t.TempDir(), time.Now()); !ok || msg != "disabled" {
		t.Fatalf("disabled: %q %v", msg, ok)
	}

	dataDir := t.TempDir()
	os.MkdirAll(filepath.Join(dataDir, "events", "pi"), 0o755)
	if msg, ok := consolidateHealth(cfg, dataDir, time.Now()); ok || !strings.Contains(msg, "never run") {
		t.Fatalf("never run: %q %v", msg, ok)
	}

	noEvents := t.TempDir()
	if msg, ok := consolidateHealth(cfg, noEvents, time.Now()); !ok || !strings.Contains(msg, "no events") {
		t.Fatalf("no events: %q %v", msg, ok)
	}

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	consolidate.Run(dataDir, consolidate.Options{Now: now.Add(-2 * time.Hour)})
	msg, ok := consolidateHealth(cfg, dataDir, now)
	if !ok || !strings.Contains(msg, "last run 2h0m0s ago") || !strings.Contains(msg, "created") {
		t.Fatalf("after run: %q %v", msg, ok)
	}
}
