package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// SearchChunks ranks `### finding` blocks rather than whole pages. A page is a
// mixed bag — muse.md carries a hundred unrelated findings — so scoring the
// block that actually answers the query beats scoring the page that happens to
// contain it. The text scored is the enriched chunk text, so the page's title
// and type count toward the match.
func SearchChunks(memoryPath, query string) ([]SearchResult, error) {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	units, err := readChunkDocs(memoryPath)
	if err != nil {
		return nil, err
	}
	c := newCorpus(units)
	weights := c.weights(terms)

	var results []SearchResult
	for _, d := range c.docs {
		score := c.score(d, weights)
		if score <= 0 {
			continue
		}
		results = append(results, SearchResult{
			Path:    d.page,
			Line:    d.line,
			Content: excerpt(d.body),
			Score:   int(score * 100),
		})
	}
	sortResults(results)
	return results, nil
}

func readChunkDocs(memoryPath string) ([]doc, error) {
	pages, err := readDocs(memoryPath)
	if err != nil {
		return nil, err
	}
	var units []doc
	for _, p := range pages {
		for _, c := range Chunks(p.path, p.body) {
			units = append(units, newUnit(ChunkKey(c), c.Path, c.Line, c.Text()))
		}
	}
	return units, nil
}

func excerpt(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "**Date**") {
			return trimmed
		}
	}
	return strings.TrimSpace(body)
}

// FuseRRF combines two rankings by reciprocal rank fusion. It fuses on rank,
// never on raw score: the lexical and dense halves are on different scales, and
// small float wobble in a score must not be able to reorder the result.
func FuseRRF(rankings [][]SearchResult, limit int) []SearchResult {
	const k = 60.0
	scores := map[string]float64{}
	seen := map[string]SearchResult{}
	for _, ranking := range rankings {
		for i, r := range ranking {
			key := r.Path + "#" + itoa(r.Line)
			scores[key] += 1.0 / (k + float64(i+1))
			if _, ok := seen[key]; !ok {
				seen[key] = r
			}
		}
	}
	fused := make([]SearchResult, 0, len(scores))
	for key, score := range scores {
		r := seen[key]
		r.Score = int(score * 100000)
		fused = append(fused, r)
	}
	sortResults(fused)
	if limit > 0 && len(fused) > limit {
		fused = fused[:limit]
	}
	return fused
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func wikiDir() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(home, ".mycelium", "memory")
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	return dir, true
}
