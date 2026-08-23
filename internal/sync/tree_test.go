package sync

import "testing"

// The pruning is a cost fix, but the way it breaks is not. Skipping the root
// would return an empty tree, and an empty tree is indistinguishable from every
// file having been deleted, so the reconciler would propagate that. This test
// fails loudly in that case: it wants the one real page back.
func TestLocalTreeSkipsDotDirectoriesWithoutSkippingTheRoot(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "memory/index.md", "# Index")
	write(t, dir, ".git/HEAD", "ref: refs/heads/main")
	write(t, dir, ".git/objects/aa/deadbeef", "object")
	write(t, dir, ".conflicts/memory/index.md", "the losing version")
	write(t, dir, "runs/suite-check/2026.json", "{}")

	entries, err := LocalTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "memory/index.md" {
		t.Fatalf("want the one real page and nothing else, got %+v", entries)
	}
}

// syncSkip already excluded every one of these file by file, so the tree came
// out right either way. What was wrong was the walk reading .git to build a
// list it then threw away, which gets worse with every commit the journal adds.
func TestSkipWalkDirPrunesWhatSyncSkipExcludes(t *testing.T) {
	for _, dir := range []string{".git", ".git/objects", ".conflicts", "runs", "runs/suite-check"} {
		if !skipWalkDir(dir) {
			t.Fatalf("%q must be pruned, not walked one file at a time", dir)
		}
	}
	for _, dir := range []string{".", "memory", "usage", "usage/ruche", "sessions/lucy"} {
		if skipWalkDir(dir) {
			t.Fatalf("%q must still be walked", dir)
		}
	}
}

// TestInstalledPackagesNeverSync guards a directory that does not exist yet.
// extensions/models carries a package.json, so one bun install inside the data
// directory would push every file of node_modules to the server and to every
// other machine. Matching the whole segment keeps a page that merely has the
// word in its name.
func TestInstalledPackagesNeverSync(t *testing.T) {
	for _, path := range []string{
		"node_modules/left-pad/index.js",
		"extensions/models/node_modules/zod/package.json",
	} {
		if !syncSkip(path) {
			t.Errorf("%q would sync", path)
		}
	}
	for _, path := range []string{
		"memory/tools/my-node_modules.md",
		"memory/conventions/node_modules-and-sync.md",
	} {
		if syncSkip(path) {
			t.Errorf("%q is a page and must sync", path)
		}
	}
	if !skipWalkDir("extensions/models/node_modules") {
		t.Error("the directory must be pruned rather than walked file by file")
	}
}
