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

	// The cross-language set is 12 cases, so one miss costs 0.083 of recall:
	// 11 hits is 0.917 and 10 is 0.833. The floor sits between them so a single
	// case drifting out of the top 5 as the corpus grows is tolerated and two
	// are not. Both floors sit far above the pre-conversion numbers (recall
	// 0.500, MRR 0.215), which is the state they exist to catch coming back.
	crossLangRecallFloor     = 0.90
	crossLangReciprocalFloor = 0.55

	// Below this share of the golden pages, the wiki on this machine is not the
	// corpus these sets were written against and the eval is skipped. The floor
	// sits far under recallFloor on purpose: a handful of renamed pages must
	// stay a loud failure that names them, and only a corpus that is essentially
	// gone — a reset, a fresh machine — becomes a skip.
	corpusFloor = 0.25
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
	dir := filepath.Join(home, ".jardin", "memory")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no wiki on this machine; eval needs ~/.jardin/memory")
	}
	return dir
}

// requireCorpus skips when the wiki here no longer holds the pages the golden
// set names. Recall measured against a corpus that does not contain the answers
// is not a low score, it is not a measurement — and a test that is red forever
// is a test nobody reads, which costs more than the coverage it pretends to add.
func requireCorpus(t *testing.T, dir string, cases []goldenCase) {
	t.Helper()
	total, present := 0, 0
	for _, c := range cases {
		for _, want := range c.Expect {
			total++
			if _, err := os.Stat(filepath.Join(dir, want)); err == nil {
				present++
			}
		}
	}
	if total == 0 {
		t.Fatal("golden set names no pages")
	}
	if share := float64(present) / float64(total); share < corpusFloor {
		t.Skipf("wiki holds %d of %d golden pages (%.0f%%, floor %.0f%%): this is not the corpus the set was written against, so the eval would measure nothing. Regenerate testdata against the current wiki to re-arm it.",
			present, total, share*100, corpusFloor*100)
	}
}

// TestGoldenSetPointsAtPagesThatExist separates a retrieval regression from a
// corpus that moved. A renamed page must fail here, naming itself, rather than
// showing up as a silent drop in recall.
func TestGoldenSetPointsAtPagesThatExist(t *testing.T) {
	dir := realWiki(t)
	requireCorpus(t, dir, loadGolden(t))
	for _, c := range loadGolden(t) {
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
// ~/.jardin/skills/scripts/wiki-english-check.ts, which scans every page line by
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

// The guard exists to skip a missing corpus, not to make the eval optional. A
// wiki that still holds its golden pages must run the eval, or a real recall
// regression would leave as quietly as a reset does.
func TestRequireCorpusOnlySkipsWhenTheCorpusIsGone(t *testing.T) {
	cases := []goldenCase{{Query: "q", Expect: []string{"a.md", "b.md", "c.md", "d.md"}}}

	full := t.TempDir()
	for _, name := range cases[0].Expect {
		if err := os.WriteFile(filepath.Join(full, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ran := false
	t.Run("corpus present", func(t *testing.T) {
		requireCorpus(t, full, cases)
		ran = true
	})
	if !ran {
		t.Error("skipped a corpus holding every golden page")
	}

	ran = false
	t.Run("corpus gone", func(t *testing.T) {
		requireCorpus(t, t.TempDir(), cases)
		ran = true
	})
	if ran {
		t.Error("ran the eval against a corpus holding none of its golden pages")
	}
}
