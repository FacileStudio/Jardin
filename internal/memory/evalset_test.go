package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEvalSet(t *testing.T, dataDir string, cases []EvalCase) {
	t.Helper()
	dir := filepath.Join(dataDir, "eval")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "golden.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePages(t *testing.T, dataDir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dataDir, "memory", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A machine with no golden set is the normal case, not a broken one. Reporting
// it as unusable would put a red cross on every install that never touches the
// ranker, which is almost all of them.
func TestInspectEvalSetAbsentIsNotAFailure(t *testing.T) {
	set, err := InspectEvalSet(t.TempDir())
	if err != nil {
		t.Fatalf("absent set returned an error: %v", err)
	}
	if set.Present {
		t.Error("reported a set that is not there")
	}
	if why := set.Unusable(); why != "" {
		t.Errorf("an absent set is not a fault, got %q", why)
	}
}

// The corpus floor has to match the eval's own guard exactly. doctor going red
// later than the eval starts skipping is the gap this check exists to close, so
// the boundary is pinned in both directions: the eval skips on share < floor,
// which means a share sitting exactly on the floor still runs.
//
// The set is sized past EvalMinCases so the case-count guard does not fire first
// and mask what this is measuring.
func TestInspectEvalSetTracksTheCorpus(t *testing.T) {
	const total = 52
	cases := make([]EvalCase, 0, total)
	names := make([]string, 0, total)
	for i := range total {
		name := "bugs/p" + itoa(i) + ".md"
		cases = append(cases, EvalCase{Query: "q" + itoa(i), Expect: []string{name}})
		names = append(names, name)
	}

	full := t.TempDir()
	writeEvalSet(t, full, cases)
	writePages(t, full, names...)
	set, err := InspectEvalSet(full)
	if err != nil {
		t.Fatal(err)
	}
	if set.Cases != total || set.Named != total || set.Found != total {
		t.Errorf("got %+v, want %d cases and %d of %d pages", set, total, total, total)
	}
	if why := set.Unusable(); why != "" {
		t.Errorf("faulted a set whose every page exists: %s", why)
	}

	onFloor := t.TempDir()
	writeEvalSet(t, onFloor, cases)
	writePages(t, onFloor, names[:13]...)
	set, err = InspectEvalSet(onFloor)
	if err != nil {
		t.Fatal(err)
	}
	if set.Found != 13 {
		t.Fatalf("found %d pages, want 13", set.Found)
	}
	if why := set.Unusable(); why != "" {
		t.Errorf("13 of 52 is exactly the %.2f floor and the eval runs there, got %q", EvalCorpusFloor, why)
	}

	gone := t.TempDir()
	writeEvalSet(t, gone, cases)
	set, err = InspectEvalSet(gone)
	if err != nil {
		t.Fatal(err)
	}
	if why := set.Unusable(); why == "" {
		t.Error("a set naming no surviving page must report a fault")
	}
}

// The eval has two guards and doctor has to model both. Before this, a set whose
// pages all existed but which held too few cases reported healthy while
// TestRetrievalRecallAtK failed outright on it, which is exactly the
// green-tick-measuring-nothing state the check was added to close.
func TestInspectEvalSetCatchesATooSmallSet(t *testing.T) {
	dir := t.TempDir()
	cases := make([]EvalCase, 0, EvalMinCases-1)
	names := make([]string, 0, EvalMinCases-1)
	for i := range EvalMinCases - 1 {
		name := "bugs/p" + itoa(i) + ".md"
		cases = append(cases, EvalCase{Query: "q", Expect: []string{name}})
		names = append(names, name)
	}
	writeEvalSet(t, dir, cases)
	writePages(t, dir, names...)

	set, err := InspectEvalSet(dir)
	if err != nil {
		t.Fatal(err)
	}
	if set.Found != set.Named {
		t.Fatalf("expected every page present, got %d of %d", set.Found, set.Named)
	}
	why := set.Unusable()
	if why == "" {
		t.Fatalf("%d cases is under the %d minimum and the eval refuses to run, but this reported healthy",
			set.Cases, EvalMinCases)
	}
	if !strings.Contains(why, "cases") {
		t.Errorf("the fault should name the case count, got %q", why)
	}
}

// A set that cannot be parsed must say so rather than read as absent, or a
// corrupted file would report the same green "not on this machine" as a machine
// that simply never had one.
func TestInspectEvalSetRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "eval"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EvalSetPath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectEvalSet(dir); err == nil {
		t.Error("a malformed golden set parsed without error")
	}
}
