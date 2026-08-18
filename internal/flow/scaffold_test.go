package flow

import (
	"os"
	"testing"
)

// TestScaffoldWritesAFlowThatParses keeps the starter file from being one an
// agent must repair before List stops erroring on it.
func TestScaffoldWritesAFlowThatParses(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	path, err := Scaffold("deploy-check")
	if err != nil {
		t.Fatal(err)
	}
	flows, err := List()
	if err != nil {
		t.Fatalf("scaffolded flow does not parse: %v", err)
	}
	if len(flows) != 1 || flows[0].Name != "deploy-check" {
		t.Fatalf("want the scaffolded flow, got %+v", flows)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("deploy-check"); err != nil {
		t.Fatal(err)
	}
}

// TestScaffoldIsNotTrust proves a flow an agent writes still has to be pinned
// by a human before it can run.
func TestScaffoldIsNotTrust(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := Scaffold("deploy-check"); err != nil {
		t.Fatal(err)
	}
	f, err := Load("deploy-check")
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := IsTrusted(f)
	if err != nil {
		t.Fatal(err)
	}
	if trusted {
		t.Fatal("a scaffolded flow must not be trusted")
	}
}

// TestScaffoldRefusesToClobberOrEscape covers the two ways the name argument
// can be hostile: an existing flow, and a path that leaves the flows dir.
func TestScaffoldRefusesToClobberOrEscape(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := Scaffold("deploy-check"); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold("deploy-check"); err == nil {
		t.Fatal("want an error on an existing flow")
	}
	for _, name := range []string{"", ".hidden", "../escape", "nested/name"} {
		if _, err := Scaffold(name); err == nil {
			t.Fatalf("want an error for name %q", name)
		}
	}

}
