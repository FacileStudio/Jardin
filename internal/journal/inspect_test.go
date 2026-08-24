package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectSaysNothingIsRecordedYet(t *testing.T) {
	dir := dataDir(t)
	health, err := Inspect(dir)
	if err != nil {
		t.Fatalf("Inspect on a bare data directory: %v", err)
	}
	if health.Started {
		t.Error("a directory with no repository reports itself as recording")
	}
}

func TestInspectReportsTheLastEntry(t *testing.T) {
	dir := dataDir(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".mycelium.yml"),
		[]byte("machine: lucy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	health, err := Inspect(dir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !health.Started {
		t.Fatal("a journal with a commit reports itself as not started")
	}
	if health.Last.Machine != "lucy" || !strings.HasPrefix(health.Last.Message, "init:") {
		t.Errorf("last entry is %+v, want the init commit by lucy", health.Last)
	}
}

// TestInspectGoesRedOnDamage is the point of the check. A commit failure is a
// warning on a sync that still succeeds, so recording can stop and nothing else
// notices. Something has to be able to say so.
func TestInspectGoesRedOnDamage(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, ".git", "objects")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(dir); err == nil {
		t.Error("a history with no objects reports itself as healthy")
	}
}
