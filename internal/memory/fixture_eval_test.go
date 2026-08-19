package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	fixtureCorpusDir  = "testdata/corpus"
	fixtureGoldenFile = "testdata/golden-fixture.json"

	// Measured on the committed corpus at the time it was written: recall@5
	// 1.000 (48/48) and MRR 0.974, over 48 queries against 30 pages. The floors
	// are those numbers less a margin — two cases may fall out of the top five,
	// and MRR may drift about 0.05 — so ordinary churn in the corpus does not
	// fail the build while a real reordering does.
	//
	// What this catches, verified by breaking the ranker on purpose: inverting
	// the sort takes recall to 0.250 and MRR to 0.199, and flattening IDF so
	// every term weighs the same takes MRR to 0.917. What it does not catch is
	// a small BM25 parameter change — removing length normalisation moved
	// nothing at all. Thirty pages is not enough corpus to grade that, and
	// pretending otherwise by tightening the floors would only produce a test
	// that fails on noise. This is a regression gate, not a quality benchmark.
	fixtureRecallFloor     = 0.95
	fixtureReciprocalFloor = 0.92
)

// TestFixtureRetrieval is the eval that always runs. The three tests above
// score the live wiki at ~/.jardin/memory, which made the gate depend on
// mutable data outside the repository: the 2026-08-19 reset deleted every page
// they name, so all three skip and CI, which has no wiki at all, never ran them
// even before that. Ranking regressions had nothing left to trip over.
//
// This one reads a corpus committed under testdata/corpus, so it measures the
// same ranker on every machine and inside CI. The live-wiki tests stay: a
// fixture cannot show what real pages and real queries do to recall, which is
// why they are worth keeping the day someone rebuilds a wiki around them.
func TestFixtureRetrieval(t *testing.T) {
	data, err := os.ReadFile(fixtureGoldenFile)
	if err != nil {
		t.Fatalf("fixture golden set unreadable: %v", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("fixture golden set is not valid JSON: %v", err)
	}
	if len(cases) < 30 {
		t.Fatalf("fixture golden set has %d cases, want at least 30", len(cases))
	}
	for _, c := range cases {
		for _, want := range c.Expect {
			if _, err := os.Stat(filepath.Join(fixtureCorpusDir, want)); err != nil {
				t.Fatalf("fixture golden set names a page that is not committed: %q (query %q)", want, c.Query)
			}
		}
	}

	hits, reciprocalSum := 0, 0.0
	for _, c := range cases {
		rank := rankOfFirstExpected(t, fixtureCorpusDir, c)
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
	t.Logf("fixture recall@%d = %.3f (%d/%d)   MRR = %.3f", evalK, recall, hits, len(cases), mrr)

	if recall < fixtureRecallFloor {
		t.Errorf("fixture recall@%d dropped to %.3f, floor is %.2f", evalK, recall, fixtureRecallFloor)
	}
	if mrr < fixtureReciprocalFloor {
		t.Errorf("fixture MRR dropped to %.3f, floor is %.2f", mrr, fixtureReciprocalFloor)
	}
}
