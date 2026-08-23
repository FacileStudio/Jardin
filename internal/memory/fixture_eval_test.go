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
	fixtureHardFile   = "testdata/golden-fixture-hard.json"

	// Measured 2026-08-24 on the committed corpus of 70 pages: recall@5 1.000
	// (48/48) and MRR 0.979 under SearchChunks, 0.983 under the page-level
	// Search. Two changes the same day pushed this number opposite ways:
	// pointing the eval at SearchChunks instead of Search took MRR from 0.974 to
	// 0.990, and growing the corpus from 30 pages to 70 took it back to 0.979,
	// because there is more to beat.
	//
	// The floors are those numbers less a margin, so ordinary churn in the
	// corpus does not fail the build while a real reordering does.
	//
	// This is a regression gate, not a quality benchmark. It catches a gross
	// reordering and does not catch a small BM25 parameter change: 70 pages on
	// 70 disjoint topics is not enough corpus to grade that, and tightening the
	// floors to pretend otherwise would only produce a test that fails on noise.
	// An earlier version of this comment quoted sabotage figures (recall 0.250,
	// MRR 0.199 for an inverted sort) that do not reproduce on any corpus or
	// entry point measured here, and did not record which of either it used.
	// They are dropped rather than left to look freshly measured.
	//
	// The hard set below, and the link set in link_eval_test.go, are the answer
	// to that limitation.
	fixtureRecallFloor     = 0.95
	fixtureReciprocalFloor = 0.92

	// The hard set holds paraphrase queries over the same corpus, mechanically
	// stripped of the rarest term they shared with their answer. Measured
	// 2026-08-24: recall@5 1.000, MRR 0.853 over 60 cases.
	//
	// Recall stays at 1.000 and that is a property of the corpus, not a bug in
	// the queries. Seventy pages spread across disjoint topics make lexical
	// retrieval easy by construction: any query naming prometheus histograms
	// finds the one page about prometheus histograms whatever words surround it.
	// Stripping terms moved MRR from 0.983 to 0.853 and moved recall not at all.
	// Making this set genuinely hard needs confusable near-duplicate pages, not
	// better queries. Grade a ranking change on MRR here, and on the link set in
	// link_eval_test.go, which is hard by construction rather than by wording
	// and sits at recall 0.000 today by design.
	hardRecallFloor     = 0.90
	hardReciprocalFloor = 0.75
)

// loadFixtureSet reads a committed case file and fails if it names a page that
// is not in the corpus. A fixture set that points at a missing page is a broken
// set, never a recall miss, and the two must not look alike.
func loadFixtureSet(t *testing.T, file string, min int) []EvalCase {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("fixture set %s unreadable: %v", file, err)
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("fixture set %s is not valid JSON: %v", file, err)
	}
	if len(cases) < min {
		t.Fatalf("fixture set %s has %d cases, want at least %d", file, len(cases), min)
	}
	for _, c := range cases {
		for _, want := range c.Expect {
			if _, err := os.Stat(filepath.Join(fixtureCorpusDir, want)); err != nil {
				t.Fatalf("fixture set names a page that is not committed: %q (query %q)", want, c.Query)
			}
		}
	}
	return cases
}

// TestFixtureRetrieval is the eval that always runs. The live-wiki tests score
// ~/.mycelium/memory, which made the gate depend on mutable data outside the
// repository: the 2026-08-19 reset deleted every page they named, so all of them
// skipped and CI, which has no wiki at all, never ran them even before that.
// Ranking regressions had nothing left to trip over.
//
// This one reads a corpus committed under testdata/corpus, so it measures the
// same ranker on every machine and inside CI. The live-wiki tests stay: a
// fixture cannot show what real pages and real queries do to recall.
func TestFixtureRetrieval(t *testing.T) {
	cases := loadFixtureSet(t, fixtureGoldenFile, 30)
	recall, mrr := scoreSet(t, fixtureCorpusDir, cases)
	t.Logf("fixture recall@%d = %.3f (%d cases)   MRR = %.3f", evalK, recall, len(cases), mrr)

	if recall < fixtureRecallFloor {
		t.Errorf("fixture recall@%d dropped to %.3f, floor is %.2f", evalK, recall, fixtureRecallFloor)
	}
	if mrr < fixtureReciprocalFloor {
		t.Errorf("fixture MRR dropped to %.3f, floor is %.2f", mrr, fixtureReciprocalFloor)
	}
}

// TestFixtureHardRetrieval grades the ranker where it can actually move. The
// easy set sits at 1.000 and can only show a regression; these queries start
// below it, so an improvement has somewhere to register.
func TestFixtureHardRetrieval(t *testing.T) {
	cases := loadFixtureSet(t, fixtureHardFile, 30)
	recall, mrr := scoreSet(t, fixtureCorpusDir, cases)
	t.Logf("fixture-hard recall@%d = %.3f (%d cases)   MRR = %.3f", evalK, recall, len(cases), mrr)

	if recall < hardRecallFloor {
		t.Errorf("hard recall@%d dropped to %.3f, floor is %.2f", evalK, recall, hardRecallFloor)
	}
	if mrr < hardReciprocalFloor {
		t.Errorf("hard MRR dropped to %.3f, floor is %.2f", mrr, hardReciprocalFloor)
	}
}

// TestFixtureRetrievalPageLevel keeps the page-level Search measured. It is not
// dead code: internal/server/content.go serves GET /api/memory/search from it,
// on an authenticated route. Pointing the rest of this package at SearchChunks
// fixed a real blind spot and would have opened the mirror image of it, leaving
// a shipping entry point with no eval at all.
//
// Measured 2026-08-24: recall@5 1.000, MRR 0.983 over the 48 easy cases.
func TestFixtureRetrievalPageLevel(t *testing.T) {
	cases := loadFixtureSet(t, fixtureGoldenFile, 30)
	recall, mrr := scoreSetWith(t, fixtureCorpusDir, cases, Search)
	t.Logf("page-level recall@%d = %.3f (%d cases)   MRR = %.3f", evalK, recall, len(cases), mrr)

	if recall < fixtureRecallFloor {
		t.Errorf("page-level recall@%d dropped to %.3f, floor is %.2f", evalK, recall, fixtureRecallFloor)
	}
	if mrr < fixtureReciprocalFloor {
		t.Errorf("page-level MRR dropped to %.3f, floor is %.2f", mrr, fixtureReciprocalFloor)
	}
}
