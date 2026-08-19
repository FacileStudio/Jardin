package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFrenchLinesReportsProseAndLeavesEnglishAlone covers the false positives
// that would make this rule unusable if it got them wrong. Two matter most.
// English prose quoting the suite's package names — porte, caisse, enveloppe —
// appears on nearly every page, and English words containing French ones
// ("less", "later", "pass" carry le, la, pas) are everywhere, so the word
// boundary has to hold or the corpus reports itself.
func TestFrenchLinesReportsProseAndLeavesEnglishAlone(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"french prose", "Ceci est une page qui doit absolument etre detectee par le script.", true},
		{"english prose", "A normal English line that must not be reported by this check at all.", false},
		{"english quoting french identifiers", "The porte kit writes into caisse and the enveloppe contract stays frozen here.", false},
		{"english words containing french words", "Unless the later pass is faster, the cluster stays in a stale state here.", false},
		{"short line, ratio meaningless", "le la des", false},
		{"exempt marker", "Ceci est une citation qui reste en francais. <!-- lang:fr -->", false},
		{"accented words the go list once lacked", "Cela reste toute information déjà transmise correctement partout.", true},
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

func writePage(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const frenchPage = "Ceci est une page qui doit absolument etre detectee par le scan.\n"

// TestScanPathsCoversOnlyWikiPagesAndSkipsTheLog pins the exemptions. log.md is
// append-only by invariant so its history is never rewritten, non-markdown and
// non-wiki paths are out of scope, and a path that no longer exists must be
// skipped rather than fail — this runs on a sync, which must not break over it.
func TestScanPathsCoversOnlyWikiPagesAndSkipsTheLog(t *testing.T) {
	dir := t.TempDir()
	writePage(t, dir, "memory/bugs/drift.md", frenchPage)
	writePage(t, dir, "memory/log.md", frenchPage)
	writePage(t, dir, "memory/notes.txt", frenchPage)
	writePage(t, dir, "flows/thing.md", frenchPage)
	writePage(t, dir, "memory/bugs/clean.md", "A perfectly ordinary English page with nothing to say.\n")

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

// TestScanWikiWalksTheCorpusAndHonoursTheSameExemptions is what doctor calls, so
// it has to agree with ScanPaths rather than become a second rule.
func TestScanWikiWalksTheCorpusAndHonoursTheSameExemptions(t *testing.T) {
	dir := t.TempDir()
	memoryDir := filepath.Join(dir, "memory")
	writePage(t, dir, "memory/bugs/drift.md", frenchPage)
	writePage(t, dir, "memory/conventions/also-drift.md", "English first line, nothing here.\n"+frenchPage)
	writePage(t, dir, "memory/log.md", frenchPage)
	writePage(t, dir, "memory/bugs/clean.md", "A perfectly ordinary English page with nothing to say.\n")

	got, err := ScanWiki(memoryDir)
	if err != nil {
		t.Fatalf("ScanWiki: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanWiki returned %d finding(s), want 2: %+v", len(got), got)
	}
	for _, f := range got {
		if f.Path == "memory/log.md" {
			t.Errorf("ScanWiki reported log.md, which is exempt")
		}
	}
}

// TestFrenchLinesIgnoresURLsAndIdentifiers guards the false positive that would
// get this rule switched off. The wiki cites French sources routinely, and a
// CNIL slug carries "la", "par" and "courrier" as fragments — splitting on every
// non-letter turns one URL into a line of French on a page whose prose is
// entirely English. Found by `jardin doctor` on the real corpus, not by review.
func TestFrenchLinesIgnoresURLsAndIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{
			"french source URLs",
			"**Source**: https://www.cnil.fr/fr/la-prospection-commerciale-par-courrier-electronique; " +
				"https://www.cnil.fr/fr/la-reutilisation-des-donnees-publiquement-accessibles-en-ligne",
		},
		{"kebab-case identifiers", "The pages facile-backups, porte-mixed-store-wiring and les-choses-ici stay put."},
		{"file paths", "Check apps/api/modules/le/la.go and internal/des/une.go before the les/la merge here."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FrenchLines(c.line); len(got) != 0 {
				t.Errorf("FrenchLines flagged %q as French; URLs and identifiers are not prose", c.line)
			}
		})
	}
}
