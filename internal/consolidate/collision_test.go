package consolidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateNewPageAppendsOnSlugCollision is the point of the guard. Two
// findings whose first lines kebab the same way land on the same filename, and
// the deduper only reports a Match above weakMatchSimilarity, so a page can
// exist that the decision knows nothing about. Overwriting it would delete a
// claim, which is the one thing this stage must never do.
func TestCreateNewPageAppendsOnSlugCollision(t *testing.T) {
	c := Candidate{
		Text:        "The fix was to pass --strip-trailing-cr to the CLI diff command or every line shows as changed.",
		EpisodeRefs: []string{"pi@2026-08-24T10:00:00Z"},
	}
	dir := tempPage(t, "bugs/the-fix-was-to-pass-strip-trailing-cr-to-the-cli.md",
		"---\ntitle: Existing\n---\n\n### a claim somebody wrote by hand\nbody worth keeping\n")
	w := &Writer{MemoryPath: dir}

	path, err := w.Create(Decision{Outcome: OutcomeCreate}, c, writeNow)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "body worth keeping") {
		t.Fatalf("the existing page was overwritten:\n%s", got)
	}
	if !strings.Contains(got, "**Source**: consolidation, pi@2026-08-24T10:00:00Z") {
		t.Fatalf("the new finding was not appended:\n%s", got)
	}
	if strings.Count(got, "title: Existing") != 1 {
		t.Fatalf("frontmatter was duplicated:\n%s", got)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "bugs", "*.tmp")); len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

// TestSupersedeStrikesEveryParagraph covers the shape a single ~~ pair cannot
// render: a claim written as two paragraphs would come out half struck and half
// standing.
func TestSupersedeStrikesEveryParagraph(t *testing.T) {
	dir := tempPage(t, "bugs/two.md",
		"### a claim in two paragraphs\nfirst paragraph of the claim\n\nsecond paragraph of the claim\n")
	w := &Writer{MemoryPath: dir}
	dec := Decision{Outcome: OutcomeSupersede, Match: &Match{Path: "bugs/two.md", Line: 1}}

	if err := w.Supersede(dec, Candidate{Text: "the correction"}, writeNow); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "bugs", "two.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"~~first paragraph of the claim~~", "~~second paragraph of the claim~~"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "~~ [SUPERSEDED by: consolidation, 2026-08-24]") {
		t.Fatalf("the supersession marker must follow the last struck span:\n%s", got)
	}
}
