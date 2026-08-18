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

// TestGoldenSetPointsAtPagesThatExist separates a retrieval regression from a
// corpus that moved. A renamed page must fail here, naming itself, rather than
// showing up as a silent drop in recall.
func TestGoldenSetPointsAtPagesThatExist(t *testing.T) {
	dir := realWiki(t)
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

// TestCrossLanguageRetrieval gauges how reachable the French half of the wiki
// is from an English query. It deliberately has no floor and never fails: it
// measures a migration in progress, not a regression. Every case here is a page
// still written in French, so the number rises as pages convert and is the
// cheapest way to see whether the English-only rule is being followed.
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
	t.Logf("cross-language recall@%d = %.3f (%d/%d)   MRR = %.3f",
		evalK, float64(hits)/n, hits, len(cases), reciprocal/n)
}
