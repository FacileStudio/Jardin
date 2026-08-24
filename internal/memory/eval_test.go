package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
)

const (
	evalK           = 5
	recallFloor     = 0.85
	reciprocalFloor = 0.75

	// Below this share of the golden pages, the wiki on this machine is not the
	// corpus these sets were written against and the eval is skipped. The floor
	// sits far under recallFloor on purpose: a handful of renamed pages must
	// stay a loud failure that names them, and only a corpus that is essentially
	// gone — a reset, a fresh machine — becomes a skip.
	//
	// It is EvalCorpusFloor rather than a second literal because doctor's eval
	// check exists to predict this skip, and a doctor that disagrees with the
	// eval reports a healthy set the eval refuses to run: the exact green tick
	// over nothing that the check was added to catch.
	corpusFloor = EvalCorpusFloor
)

// goldenSetPath resolves the live-wiki golden set, which lives under the data
// directory rather than in this repository. This repository is public and the
// set is a plain-English description of every page in a private wiki, so
// committing it would publish that for the benefit of nobody: the eval only runs
// where a wiki exists, and CI has none. Under the data directory it reaches the
// machines that can use it by the same sync the wiki already uses.
//
// It resolves through config.DataDir, which honours DATA_DIR, so this and the
// eval set check in doctor always read the same file. Hardcoding a home
// directory here would let doctor report on one tree while the eval measured
// another.
func goldenSetPath() (string, bool) {
	path := EvalSetPath(config.DataDir())
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

func loadGolden(t *testing.T) []EvalCase {
	t.Helper()
	path, ok := goldenSetPath()
	if !ok {
		t.Skipf("no golden set at %s; the live-wiki eval runs only where the wiki does", EvalSetPath(config.DataDir()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden set unreadable: %v", err)
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("golden set is not valid JSON: %v", err)
	}
	if len(cases) < EvalMinCases {
		t.Fatalf("golden set has %d cases, want at least %d", len(cases), EvalMinCases)
	}
	return cases
}

func realWiki(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(config.DataDir(), "memory")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no wiki on this machine; the eval needs a memory directory under the data dir")
	}
	return dir
}

// requireCorpus skips when the wiki here no longer holds the pages the golden
// set names. Recall measured against a corpus that does not contain the answers
// is not a low score, it is not a measurement — and a test that is red forever
// is a test nobody reads, which costs more than the coverage it pretends to add.
func requireCorpus(t *testing.T, dir string, cases []EvalCase) {
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

// TestRetrievalRecallAtK is the live-corpus regression gate. Measured
// 2026-08-24 on the regenerated set: recall@5 1.000 and MRR around 0.98 over 76
// cases naming all 50 pages. The MRR is deliberately imprecise here, because it
// moves every time a page is written: it read 0.978 and 0.980 twenty minutes
// apart on the day it was recorded. Read the log line, not this comment. The floors sit well under that on purpose, because this
// corpus is the live wiki and gains pages every week; a floor set close to
// today's number would go red because somebody wrote a page rather than because
// ranking moved. corpusFloor catches the other failure, a corpus that is gone.
//
// Read the 1.000 as a warning, not a triumph. Each query was written by reading
// the page it names and checked against live search, which selects for queries
// the ranker already answers, so this set can show a regression and cannot show
// an improvement. It is not the place to grade a ranking change. The fixture
// link set is, because its cases are built to sit outside the top five.
func TestRetrievalRecallAtK(t *testing.T) {
	dir := realWiki(t)
	cases := loadGolden(t)
	requireCorpus(t, dir, cases)

	recall, mrr := scoreSet(t, dir, cases)
	t.Logf("recall@%d = %.3f (%d cases)   MRR = %.3f", evalK, recall, len(cases), mrr)

	if recall < recallFloor {
		t.Errorf("recall@%d dropped to %.3f, floor is %.2f", evalK, recall, recallFloor)
	}
	if mrr < reciprocalFloor {
		t.Errorf("MRR dropped to %.3f, floor is %.2f", mrr, reciprocalFloor)
	}
}

// searcher is a retrieval entry point. Both shipping ones are measured: the
// eval used to grade only the page-level Search, and pointing it at SearchChunks
// alone would have moved the blind spot rather than removed it.
type searcher func(dir, query string) ([]SearchResult, error)

// scoreSet runs every case through SearchChunks, the chunk-level path the CLI
// and the POST route use.
func scoreSet(t *testing.T, dir string, cases []EvalCase) (recall, mrr float64) {
	t.Helper()
	return scoreSetWith(t, dir, cases, SearchChunks)
}

// scoreSetWith runs every case through search and returns recall@evalK and MRR.
// Each miss is logged with its query, because a recall number on its own does
// not say which case moved.
func scoreSetWith(t *testing.T, dir string, cases []EvalCase, search searcher) (recall, mrr float64) {
	t.Helper()
	hits, reciprocal := 0, 0.0
	for _, c := range cases {
		rank := rankWith(t, dir, c, search)
		if rank > 0 && rank <= evalK {
			hits++
		}
		if rank > 0 {
			reciprocal += 1.0 / float64(rank)
		} else {
			t.Logf("miss: %q → want one of %v", c.Query, c.Expect)
		}
	}
	n := float64(len(cases))
	return float64(hits) / n, reciprocal / n
}

// rankOfFirstExpected returns the rank of the first expected page under
// SearchChunks, the chunk-level path. cmd/memory.go and the POST half of
// /api/memory/search both call it, and step 10's recency decay and struck-span
// dropping live there and nowhere else.
//
// It is not the only shipping path. internal/server/content.go serves
// GET /api/memory/search from the page-level Search, so both are measured; see
// TestFixtureRetrievalPageLevel.
func rankOfFirstExpected(t *testing.T, dir string, c EvalCase) int {
	t.Helper()
	return rankWith(t, dir, c, SearchChunks)
}

// rankWith returns the 1-based rank of the first expected page under search, or
// 0 when it does not appear at all.
func rankWith(t *testing.T, dir string, c EvalCase, search searcher) int {
	t.Helper()
	results, err := search(dir, c.Query)
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

// The guard exists to skip a missing corpus, not to make the eval optional. A
// wiki that still holds its golden pages must run the eval, or a real recall
// regression would leave as quietly as a reset does.
func TestRequireCorpusOnlySkipsWhenTheCorpusIsGone(t *testing.T) {
	cases := []EvalCase{{Query: "q", Expect: []string{"a.md", "b.md", "c.md", "d.md"}}}

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
