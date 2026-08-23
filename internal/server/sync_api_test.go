package server

import "testing"

// TestServerSkipsInstalledPackagesToo keeps the two copies of this rule in
// step. A path the client excludes and the server does not is a file the
// reconcile sees on one side only, which it reads as a deletion.
func TestServerSkipsInstalledPackagesToo(t *testing.T) {
	for _, path := range []string{
		"node_modules/left-pad/index.js",
		"extensions/models/node_modules/zod/package.json",
	} {
		if !syncSkip(path) {
			t.Errorf("%q would sync", path)
		}
	}
	if syncSkip("memory/tools/my-node_modules.md") {
		t.Error("a page with the word in its name must still sync")
	}
}
