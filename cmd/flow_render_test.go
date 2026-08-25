package cmd

import (
	"testing"

	"github.com/FacileStudio/Mycelium/internal/flow"
)

// The rendered table is covered through flowRecap; this covers the other half
// of the same file. "mycelium flow list --json" is read by scripts rather than
// people, and its trust field is their only machine-readable answer to "would
// this run here?", so the strings are asserted literally.
func TestFlowListRowsCarryTheTrustVocabularyScriptsParse(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	pinned := writeFlow(t, "pinned", "name: pinned\nsteps:\n  - name: a\n    run: 'true'\n")
	edited := writeFlow(t, "edited", "name: edited\nsteps:\n  - name: a\n    run: 'true'\n")
	for _, f := range []*flow.Flow{pinned, edited} {
		if err := flow.Trust(f); err != nil {
			t.Fatal(err)
		}
	}
	edited = writeFlow(t, "edited", "name: edited\nsteps:\n  - name: a\n    run: 'false'\n")
	unpinned := writeFlow(t, "unpinned", "name: unpinned\nsteps:\n  - name: a\n    run: 'true'\n")

	want := map[string]string{"pinned": "trusted", "edited": "CHANGED", "unpinned": "not pinned"}
	rows := flowListRows([]*flow.Flow{pinned, edited, unpinned})
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for _, row := range rows {
		if row.Trust != want[row.Name] {
			t.Errorf("%s trust = %q, want %q", row.Name, row.Trust, want[row.Name])
		}
	}
}
