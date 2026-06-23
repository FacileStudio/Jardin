package cell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRulesOrderIsPreferenceAndTolerant(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	rules := filepath.Join(dir, "rules")
	if err := os.MkdirAll(rules, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if err := os.WriteFile(filepath.Join(rules, n+".md"), []byte("# "+n), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ReadRules([]string{"gamma", "missing", "alpha"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, f := range got {
		names = append(names, f.Name)
	}
	want := []string{"gamma", "alpha", "beta"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
}

func TestReadRulesNoOrderIsAlphabetical(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	rules := filepath.Join(dir, "rules")
	os.MkdirAll(rules, 0755)
	for _, n := range []string{"20-c", "00-a", "10-b"} {
		os.WriteFile(filepath.Join(rules, n+".md"), []byte("x"), 0644)
	}
	got, _ := ReadRules(nil)
	if len(got) != 3 || got[0].Name != "00-a" || got[2].Name != "20-c" {
		t.Fatalf("expected prefix-sorted order, got %v", got)
	}
}
