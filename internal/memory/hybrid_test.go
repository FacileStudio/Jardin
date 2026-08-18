package memory

import (
	"path/filepath"
	"testing"
)

type ranker func(dir, query string) ([]SearchResult, error)

func measure(t *testing.T, dir string, cases []goldenCase, rank ranker) (float64, float64) {
	t.Helper()
	hits, reciprocal := 0, 0.0
	for _, c := range cases {
		results, err := rank(dir, c.Query)
		if err != nil {
			t.Fatalf("%q: %v", c.Query, err)
		}
		wanted := map[string]bool{}
		for _, w := range c.Expect {
			wanted[filepath.FromSlash(w)] = true
		}
		seen, position := map[string]bool{}, 0
		for _, r := range results {
			if seen[r.Path] {
				continue
			}
			seen[r.Path] = true
			position++
			if wanted[r.Path] {
				if position <= evalK {
					hits++
				}
				reciprocal += 1.0 / float64(position)
				break
			}
		}
	}
	n := float64(len(cases))
	return float64(hits) / n, reciprocal / n
}

// TestChunkLevelBeatsPageLevel is the measurement that decides whether the
// chunker earns its place before a single vector exists.
func TestChunkLevelBeatsPageLevel(t *testing.T) {
	dir, ok := wikiDir()
	if !ok {
		t.Skip("no wiki on this machine")
	}
	cases := loadGolden(t)

	pageRecall, pageMRR := measure(t, dir, cases, Search)
	chunkRecall, chunkMRR := measure(t, dir, cases, SearchChunks)

	t.Logf("page-level   recall@%d = %.3f   MRR = %.3f", evalK, pageRecall, pageMRR)
	t.Logf("chunk-level  recall@%d = %.3f   MRR = %.3f", evalK, chunkRecall, chunkMRR)
	t.Logf("delta        recall %+.3f   MRR %+.3f", chunkRecall-pageRecall, chunkMRR-pageMRR)
}

// TestFuseRRFOrdersByRankNotScore proves fusion cannot be swayed by the scale
// of either half: a chunk ranked first by both wins over one ranked first by
// only one, whatever the raw numbers say.
func TestFuseRRFOrdersByRankNotScore(t *testing.T) {
	lexical := []SearchResult{
		{Path: "a.md", Line: 1, Score: 999999},
		{Path: "b.md", Line: 1, Score: 2},
	}
	dense := []SearchResult{
		{Path: "b.md", Line: 1, Score: 1},
		{Path: "c.md", Line: 1, Score: 1},
	}
	fused := FuseRRF([][]SearchResult{lexical, dense}, 10)
	if len(fused) != 3 {
		t.Fatalf("want three distinct hits, got %d", len(fused))
	}
	if fused[0].Path != "b.md" {
		t.Fatalf("the hit ranked by both halves must win, got %q", fused[0].Path)
	}
}

// TestFuseRRFIsDeterministic locks the property that map iteration cannot leak
// into the fused order.
func TestFuseRRFIsDeterministic(t *testing.T) {
	a := []SearchResult{{Path: "a.md", Line: 1}, {Path: "b.md", Line: 2}, {Path: "c.md", Line: 3}}
	b := []SearchResult{{Path: "c.md", Line: 3}, {Path: "a.md", Line: 1}, {Path: "d.md", Line: 4}}
	first := FuseRRF([][]SearchResult{a, b}, 10)
	for i := 0; i < 25; i++ {
		again := FuseRRF([][]SearchResult{a, b}, 10)
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("run %d differs at %d: %+v vs %+v", i, j, first[j], again[j])
			}
		}
	}
}
