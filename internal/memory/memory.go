package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const linesPerFile = 3

// SearchResult is one matching line from a memory file. Score ranks it against
// the other results for the same query; it is not comparable across queries.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	Score   int    `json:"score"`
}

// Search returns memory lines matching a query, best first. Pages are ranked
// with BM25 over the whole query, so a paraphrase still finds its page: a term
// present in nearly every page contributes almost nothing, a rare identifier
// dominates, and a page missing some terms still ranks. A term matches as a
// prefix, so "trust" finds "trusted".
func Search(memoryPath, query string) ([]SearchResult, error) {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	docs, err := readDocs(memoryPath)
	if err != nil {
		return nil, err
	}
	c := newCorpus(docs)
	weights := c.weights(terms)

	var results []SearchResult
	for _, d := range c.docs {
		if c.score(d, weights) <= 0 {
			continue
		}
		results = append(results, bestLines(d, weights, linesPerFile)...)
	}
	sortResults(results)
	return results, nil
}

func queryTerms(query string) []string {
	seen := make(map[string]bool)
	var terms []string
	for _, tok := range tokenize(query) {
		if seen[tok] {
			continue
		}
		seen[tok] = true
		terms = append(terms, tok)
	}
	return terms
}

func readDocs(memoryPath string) ([]doc, error) {
	var docs []doc
	err := filepath.Walk(memoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(memoryPath, path)
		docs = append(docs, newDoc(rel, string(data)))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", memoryPath, err)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].path < docs[j].path })
	return docs, nil
}

func sortResults(results []SearchResult) {
	sort.SliceStable(results, func(i, j int) bool { return better(results[i], results[j]) })
}

func better(a, b SearchResult) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Line < b.Line
}

// ReadIndex returns the memory index.md file.
func ReadIndex(memoryPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(memoryPath, "index.md"))
	if err != nil {
		return "", fmt.Errorf("memory index not found: %w", err)
	}
	return string(data), nil
}
