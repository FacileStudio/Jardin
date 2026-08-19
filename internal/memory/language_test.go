package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrenchLinesReportsProseAndLeavesEnglishAlone(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"french prose", "Ceci est une page qui doit absolument etre detectee par le script.", true},
		{"english prose", "A normal English line that must not be reported by this check at all.", false},
		{
			// porte, caisse and enveloppe are suite package names and appear
			// constantly in English sentences.
			"english quoting french identifiers",
			"The porte kit writes into caisse and the enveloppe contract stays frozen here.",
			false,
		},
		{
			// "less", "later" and "pass" contain le/la/pas; the word boundary
			// has to hold or every English page reports itself.
			"english words containing french words",
			"Unless the later pass is faster, the cluster stays in a stale state here.",
			false,
		},
		{"short line, ratio meaningless", "le la des", false},
		{"exempt marker", "Ceci est une citation qui reste en francais. <!-- lang:fr -->", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := len(FrenchLines(c.line)) > 0
			if got != c.want {
				t.Errorf("FrenchLines(%q) french=%v, want %v", c.line, got, c.want)
			}
		})
	}
}

func TestFrenchLinesReportsEveryOffendingLineNumber(t *testing.T) {
	content := "An English opening line that says nothing of interest here.\n" +
		"Ceci est une ligne qui doit absolument etre detectee sans faute.\n" +
		"Another English line, still nothing to report on this one.\n" +
		"Une deuxieme ligne en francais qui est ecrite pour la verification.\n"
	got := FrenchLines(content)
	want := []int{2, 4}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FrenchLines returned %v, want %v", got, want)
	}
}

func TestScanPathsCoversOnlyWikiPagesAndSkipsTheLog(t *testing.T) {
	dir := t.TempDir()
	french := "Ceci est une page qui doit absolument etre detectee par le scan.\n"

	write := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("memory/bugs/drift.md", french)
	write("memory/log.md", french)    // append-only, exempt
	write("memory/notes.txt", french) // not markdown
	write("flows/thing.md", french)   // not the wiki
	write("memory/bugs/clean.md", "A perfectly ordinary English page with nothing to say.\n")

	rels := []string{
		"memory/bugs/drift.md", "memory/log.md", "memory/notes.txt",
		"flows/thing.md", "memory/bugs/clean.md", "memory/bugs/missing.md",
	}
	got := ScanPaths(dir, rels)
	if len(got) != 1 {
		t.Fatalf("ScanPaths returned %d finding(s), want 1: %+v", len(got), got)
	}
	if got[0].Path != "memory/bugs/drift.md" || got[0].Line != 1 {
		t.Errorf("ScanPaths returned %+v, want memory/bugs/drift.md:1", got[0])
	}
}
