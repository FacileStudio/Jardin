package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	evalK           = 5
	goldenSetFile   = "testdata/golden.json"
	recallFloor     = 0.60
	reciprocalFloor = 0.50

	// A quarter of the golden pages is the line between "the corpus moved" and
	// "the corpus is gone". Editing ranking code never deletes three quarters of
	// the wiki, so anything above this floor is still a real measurement and
	// stays a failure; below it the eval is scoring an empty shelf.
	corpusFloor = 0.25

	// The cross-language set is 12 cases, so one miss costs 0.083 of recall:
	// 11 hits is 0.917 and 10 is 0.833. The floor sits between them so a single
	// case drifting out of the top 5 as the corpus grows is tolerated and two
	// are not. Both floors sit far above the pre-conversion numbers (recall
	// 0.500, MRR 0.215), which is the state they exist to catch coming back.
	crossLangRecallFloor     = 0.90
	crossLangReciprocalFloor = 0.55
)

type goldenCase struct {
	Query  string   `json:"query"`
	Expect []string `json:"expect"`
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	data, err := os.ReadFile(goldenSetFile)
	if err != nil {
		t.Fatalf("golden set unreadable: %v", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("golden set is not valid JSON: %v", err)
	}
	if len(cases) < 50 {
		t.Fatalf("golden set has %d cases, want at least 50", len(cases))
	}
	return cases
}

func realWiki(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir := filepath.Join(home, ".mycelium", "memory")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no wiki on this machine; eval needs ~/.mycelium/memory")
	}
	return dir
}

// requireCorpus extends realWiki's skip philosophy from "no wiki" to "no corpus
// to measure". The eval scores ranking against the live wiki, so a machine that
// reset or rebuilt ~/.mycelium/memory has nothing to score and would otherwise
// fail the build until 65 named pages were recreated by hand. Skipping needs a
// wholesale-missing corpus: at corpusFloor a rename or five still trips
// TestGoldenSetPointsAtPagesThatExist loudly, which is the signal that tells a
// moved page apart from a recall drop.
func requireCorpus(t *testing.T, dir string, cases []goldenCase) {
	t.Helper()
	pages := map[string]bool{}
	for _, c := range cases {
		for _, want := range c.Expect {
			pages[want] = true
		}
	}
	present := 0
	for page := range pages {
		if _, err := os.Stat(filepath.Join(dir, page)); err == nil {
			present++
		}
	}
	if len(pages) == 0 || float64(present)/float64(len(pages)) < corpusFloor {
		t.Skipf("golden corpus absent from %s (%d/%d pages present) — wiki reset or rebuilt, nothing to measure", dir, present, len(pages))
	}
}

// TestGoldenSetPointsAtPagesThatExist separates a retrieval regression from a
// corpus that moved. A renamed page must fail here, naming itself, rather than
// showing up as a silent drop in recall.
func TestGoldenSetPointsAtPagesThatExist(t *testing.T) {
	dir := realWiki(t)
	cases := loadGolden(t)
	requireCorpus(t, dir, cases)
	for _, c := range cases {
		for _, want := range c.Expect {
			if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
				t.Errorf("golden set references a missing page %q (query %q) — fix the golden set, this is not a recall miss", want, c.Query)
			}
		}
	}
}

// TestRetrievalRecallAtK is the gate every retrieval change is measured
// against. It prints recall@5 and MRR so a change can be compared to the
// number the last run reported.
func TestRetrievalRecallAtK(t *testing.T) {
	dir := realWiki(t)
	cases := loadGolden(t)
	requireCorpus(t, dir, cases)

	hits, reciprocalSum := 0, 0.0
	for _, c := range cases {
		rank := rankOfFirstExpected(t, dir, c)
		if rank > 0 && rank <= evalK {
			hits++
		}
		if rank > 0 {
			reciprocalSum += 1.0 / float64(rank)
		} else {
			t.Logf("miss: %q → want one of %v", c.Query, c.Expect)
		}
	}

	recall := float64(hits) / float64(len(cases))
	mrr := reciprocalSum / float64(len(cases))
	t.Logf("recall@%d = %.3f (%d/%d)   MRR = %.3f", evalK, recall, hits, len(cases), mrr)

	if recall < recallFloor {
		t.Errorf("recall@%d dropped to %.3f, floor is %.2f", evalK, recall, recallFloor)
	}
	if mrr < reciprocalFloor {
		t.Errorf("MRR dropped to %.3f, floor is %.2f", mrr, reciprocalFloor)
	}
}

func rankOfFirstExpected(t *testing.T, dir string, c goldenCase) int {
	t.Helper()
	results, err := Search(dir, c.Query)
	if err != nil {
		t.Fatalf("search %q: %v", c.Query, err)
	}
	wanted := make(map[string]bool, len(c.Expect))
	for _, w := range c.Expect {
		wanted[filepath.FromSlash(w)] = true
	}
	rank, seen := 0, map[string]bool{}
	for _, r := range results {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		rank++
		if wanted[r.Path] {
			return rank
		}
	}
	return 0
}

const crossLangFile = "testdata/golden-crosslang.json"

// TestCrossLanguageRetrieval keeps the converted pages reachable from an
// English query. Every case here names a page that was French until the
// 2026-08-19 conversion, when recall@5 went 0.500 -> 1.000 and MRR 0.215 ->
// 0.778; the floors below guard that gain.
//
// What this does NOT do is detect new French drift. The set is a fixed list of
// twelve query->page pairs, so a French page written next month is not in it and
// this test never looks at it. That job belongs to
// ~/.mycelium/skills/scripts/wiki-english-check.ts, which scans every page line by
// line. Do not widen this test to cover it — a golden set measures ranking, not
// corpus hygiene.
func TestCrossLanguageRetrieval(t *testing.T) {
	dir, ok := wikiDir()
	if !ok {
		t.Skip("no wiki on this machine")
	}
	data, err := os.ReadFile(crossLangFile)
	if err != nil {
		t.Fatalf("cross-language set unreadable: %v", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("cross-language set is not valid JSON: %v", err)
	}
	requireCorpus(t, dir, cases)

	hits, reciprocal := 0, 0.0
	for _, c := range cases {
		rank := rankOfFirstExpected(t, dir, c)
		if rank > 0 && rank <= evalK {
			hits++
		}
		if rank > 0 {
			reciprocal += 1.0 / float64(rank)
		} else {
			t.Logf("unreachable in English: %q → %v", c.Query, c.Expect)
		}
	}
	n := float64(len(cases))
	recall, mrr := float64(hits)/n, reciprocal/n
	t.Logf("cross-language recall@%d = %.3f (%d/%d)   MRR = %.3f",
		evalK, recall, hits, len(cases), mrr)

	if recall < crossLangRecallFloor {
		t.Errorf("cross-language recall@%d dropped to %.3f, floor is %.2f",
			evalK, recall, crossLangRecallFloor)
	}
	if mrr < crossLangReciprocalFloor {
		t.Errorf("cross-language MRR dropped to %.3f, floor is %.2f",
			mrr, crossLangReciprocalFloor)
	}
}
