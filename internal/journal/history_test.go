package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeNormalisesAPageAsSearchReportsIt(t *testing.T) {
	cases := map[string]string{
		"":                       "",
		"conventions/no-slop.md": "memory/conventions/no-slop.md",
		"memory/tools/filet.md":  "memory/tools/filet.md",
		"./bugs":                 "memory/bugs",
		"flows":                  "flows",
		"skills/muse/SKILL.md":   "skills/muse/SKILL.md",
	}
	for in, want := range cases {
		if got := Scope(in); got != want {
			t.Errorf("Scope(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLogOnAMachineWithNoHistory covers the upgrade path: a client that has
// synced for months has no repository until something commits, and asking for
// the history there is an empty answer rather than a failure.
func TestLogOnAMachineWithNoHistory(t *testing.T) {
	dir := dataDir(t)
	entries, err := Log(dir, "", 10)
	if err != nil {
		t.Fatalf("Log on a bare data directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
}

func TestLogNarrowsToOnePage(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "memory/tools/filet.md", "### one\n")
	if err := Commit(dir, "sync: pulled 1"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "memory/bugs/other.md", "### two\n")
	if err := Commit(dir, "sync: pulled 1 more"); err != nil {
		t.Fatal(err)
	}

	all, err := Log(dir, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d entries for the whole tree, want 3", len(all))
	}
	one, err := Log(dir, "tools/filet.md", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Message != "sync: pulled 1" {
		t.Errorf("got %+v, want only the commit that touched that page", one)
	}
}

func TestLogRespectsTheLimit(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		write(t, dir, "memory/index.md", strings.Repeat("x\n", i+1))
		if err := Commit(dir, "sync: pulled 1"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := Log(dir, "", 2)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}

// TestRevertRestoresADeletedPage is the exit criterion of the whole track,
// asserted through the surface a human actually uses.
func TestRevertRestoresADeletedPage(t *testing.T) {
	dir := dataDir(t)
	write(t, dir, "memory/tools/filet.md", "### the gate is failOn error\n")
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	before, err := Log(dir, "", 1)
	if err != nil || len(before) == 0 {
		t.Fatalf("no history to revert to: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "memory/tools/filet.md")); err != nil {
		t.Fatal(err)
	}
	if err := Commit(dir, "sync: removed 1 here"); err != nil {
		t.Fatal(err)
	}
	if err := Revert(dir, before[0].Ref, "tools/filet.md"); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "memory/tools/filet.md"))
	if err != nil {
		t.Fatalf("the page was not restored: %v", err)
	}
	if !strings.Contains(string(body), "failOn error") {
		t.Errorf("restored %q, want the page's text", body)
	}
}

// TestRevertRemovesWhatDidNotExistThen is the half a restore-only revert gets
// wrong: undoing a sync that added a page has to take the page away again, or
// the tree matches no state that ever existed.
func TestRevertRemovesWhatDidNotExistThen(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	base, err := Log(dir, "", 1)
	if err != nil || len(base) == 0 {
		t.Fatalf("no history: %v", err)
	}
	write(t, dir, "memory/bugs/spurious.md", "### not real\n")
	if err := Commit(dir, "sync: pulled 1"); err != nil {
		t.Fatal(err)
	}
	if err := Revert(dir, base[0].Ref, "bugs"); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "memory/bugs/spurious.md")); !os.IsNotExist(err) {
		t.Error("the page added after the ref is still there")
	}
}

// TestRevertIsItselfRevertible keeps a wrong ref from turning one lost page
// into two: the state a revert replaces is recorded before it is replaced.
func TestRevertIsItselfRevertible(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	base, err := Log(dir, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "memory/index.md", "# Index\n\nwritten but never synced\n")
	if err := Revert(dir, base[0].Ref, ""); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	entries, err := Log(dir, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("got %d entries, want the unsynced edit recorded before the revert", len(entries))
	}
	if !strings.HasPrefix(entries[1].Message, "local:") {
		t.Errorf("the commit before the revert is %q, want the unsynced edit", entries[1].Message)
	}
}

// TestDiffNamesNoStorage holds the line the agent surface draws: the history is
// reachable, and nothing it prints says what it is kept in.
func TestDiffNamesNoStorage(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	base, err := Log(dir, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "memory/tools/filet.md", "### a finding\n")
	if err := Commit(dir, "sync: pulled 1"); err != nil {
		t.Fatal(err)
	}
	patch, err := Diff(dir, base[0].Ref, "")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(patch, "a finding") {
		t.Errorf("the patch does not show the change: %q", patch)
	}
	if strings.Contains(patch, "git") {
		t.Errorf("the patch names the storage: %q", patch)
	}
}
