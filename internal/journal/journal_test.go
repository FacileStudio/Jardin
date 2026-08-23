package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func dataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	for _, sub := range []string{"memory", "rules", "events/lucy", "usage", "runs"} {
		if err := os.MkdirAll(filepath.Join(dir, "store", sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := filepath.Join(dir, "store")
	write(t, store, "memory/index.md", "# Index\n")
	write(t, store, "events/lucy/tick.jsonl", "{}\n")
	write(t, store, "usage/current.json", "{}\n")
	return store
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func history(t *testing.T, dir string) []*object.Commit {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("no repository at %s: %v", dir, err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	var commits []*object.Commit
	if err := iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return commits
}

func TestInitSnapshotsTheAuthoredTree(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("no repository created: %v", err)
	}
	commits := history(t, dir)
	if len(commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(commits))
	}
	if !strings.HasPrefix(commits[0].Message, "init:") {
		t.Errorf("first commit is %q, want an init message", commits[0].Message)
	}
}

// TestInitIsIdempotent guards the bootstrap path: Commit creates the
// repository when it is absent, so an install that never runs `mycelium init`
// again still gets a history, and running init twice must not add an empty
// commit to it.
func TestInitIsIdempotent(t *testing.T) {
	dir := dataDir(t)
	for range 3 {
		if err := Init(dir); err != nil {
			t.Fatalf("Init: %v", err)
		}
	}
	if n := len(history(t, dir)); n != 1 {
		t.Fatalf("got %d commits after three inits, want 1", n)
	}
}

// TestTelemetryIsNeverCommitted is the reason the journal is worth having at
// all: events, usage and runs churn on a timer, and a history where every page
// is buried under a thousand of their commits answers nothing.
func TestTelemetryIsNeverCommitted(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tracked := treePaths(t, dir)
	for _, path := range tracked {
		for _, banned := range []string{"events/", "usage/", "runs/", "claims/"} {
			if strings.HasPrefix(path, banned) {
				t.Errorf("%s was committed; telemetry must stay out of the history", path)
			}
		}
	}
	if !contains(tracked, "memory/index.md") {
		t.Errorf("memory/index.md is not in the tree, got %v", tracked)
	}
}

// TestTelemetryLeavesTheStatusClean is the other half: an uncommitted file that
// keeps showing up as changed makes every later commit look like it has work to
// do, which is how an empty commit per sync would get written.
func TestTelemetryLeavesTheStatusClean(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	write(t, dir, "events/lucy/tick.jsonl", "{}\n{}\n")
	write(t, dir, "runs/flow/1.json", "{}\n")
	before := len(history(t, dir))
	if err := Commit(dir, "sync: nothing authored changed"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if after := len(history(t, dir)); after != before {
		t.Errorf("telemetry churn produced %d new commit(s)", after-before)
	}
}

func TestCommitRecordsAnEditAndADeletion(t *testing.T) {
	dir := dataDir(t)
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	write(t, dir, "memory/bugs/one.md", "### a finding\n")
	if err := Commit(dir, "sync: pulled 1"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !contains(treePaths(t, dir), "memory/bugs/one.md") {
		t.Fatal("the new page was not committed")
	}

	if err := os.Remove(filepath.Join(dir, "memory/bugs/one.md")); err != nil {
		t.Fatal(err)
	}
	if err := Commit(dir, "sync: removed 1 here"); err != nil {
		t.Fatalf("Commit after delete: %v", err)
	}
	if contains(treePaths(t, dir), "memory/bugs/one.md") {
		t.Error("the deleted page is still in the tree; a deletion was not staged")
	}
	if n := len(history(t, dir)); n != 3 {
		t.Fatalf("got %d commits, want 3", n)
	}
}

// TestDeletedPageIsRecoverable is the exit criterion this whole track exists
// for, asserted at the storage layer: the bytes of a page removed by a sync are
// still reachable from the commit before it.
func TestDeletedPageIsRecoverable(t *testing.T) {
	dir := dataDir(t)
	write(t, dir, "memory/tools/filet.md", "### the gate is failOn error\n")
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "memory/tools/filet.md")); err != nil {
		t.Fatal(err)
	}
	if err := Commit(dir, "sync: removed 1 here"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	commits := history(t, dir)
	previous := commits[len(commits)-1]
	file, err := previous.File("memory/tools/filet.md")
	if err != nil {
		t.Fatalf("the page is not in the previous commit: %v", err)
	}
	body, err := file.Contents()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "failOn error") {
		t.Errorf("recovered %q, want the page's text", body)
	}
}

func TestCommitNamesTheMachine(t *testing.T) {
	dir := dataDir(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".mycelium.yml"),
		[]byte("machine: lucy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := history(t, dir)[0].Author.Name; got != "lucy" {
		t.Errorf("author is %q, want lucy", got)
	}
}

func treePaths(t *testing.T, dir string) []string {
	t.Helper()
	commits := history(t, dir)
	if len(commits) == 0 {
		return nil
	}
	tree, err := commits[0].Tree()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := tree.Files().ForEach(func(f *object.File) error {
		paths = append(paths, f.Name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

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

// TestVersionedRootsCoverEveryAuthoredDirectory is a reminder rather than a
// behaviour test. A new directory under the data root is either telemetry, and
// belongs in the ignore file, or authored, and belongs here. Adding one and
// deciding neither is how extensions/ went unprotected for two releases.
func TestVersionedRootsCoverEveryAuthoredDirectory(t *testing.T) {
	want := map[string]bool{"memory": true, "rules": true, "skills": true, "flows": true, "extensions": true}
	got := map[string]bool{}
	for _, root := range versionedRoots() {
		got[root] = true
		if strings.Contains(ignoreBody(), "/"+root+"/") {
			t.Errorf("%s is both versioned and ignored, so it will never be staged", root)
		}
	}
	for root := range want {
		if !got[root] {
			t.Errorf("%s is no longer versioned", root)
		}
	}
}
