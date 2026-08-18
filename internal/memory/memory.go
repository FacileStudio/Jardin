package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	distinctTermWeight = 10
	headingBonus       = 3
	linesPerFile       = 3
)

// SearchResult is one matching line from a memory file. Score ranks it against
// the other results for the same query; it is not comparable across queries.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	Score   int    `json:"score"`
}

// Search returns memory lines matching a query, best first. Every term must
// appear somewhere in a file for that file to match, in any order, and the
// lines returned are the ones carrying the most terms. A term matches inside a
// longer word, so "trust" finds "trusted" and ".flow-trust.json".
func Search(memoryPath, query string) ([]SearchResult, error) {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	var results []SearchResult
	err := filepath.Walk(memoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(memoryPath, path)
		results = append(results, searchFile(rel, string(data), terms)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search %s: %w", memoryPath, err)
	}
	sort.SliceStable(results, func(i, j int) bool { return better(results[i], results[j]) })
	return results, nil
}

func queryTerms(query string) []string {
	seen := make(map[string]bool)
	var terms []string
	for _, field := range strings.Fields(strings.ToLower(query)) {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func searchFile(rel, content string, terms []string) []SearchResult {
	if !containsAll(strings.ToLower(content), terms) {
		return nil
	}
	var hits []SearchResult
	for i, line := range strings.Split(content, "\n") {
		score := scoreLine(strings.ToLower(line), terms)
		if score == 0 {
			continue
		}
		hits = append(hits, SearchResult{
			Path:    rel,
			Line:    i + 1,
			Content: strings.TrimSpace(line),
			Score:   score,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool { return better(hits[i], hits[j]) })
	if len(hits) > linesPerFile {
		hits = hits[:linesPerFile]
	}
	return hits
}

func containsAll(lowerContent string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(lowerContent, term) {
			return false
		}
	}
	return true
}

func scoreLine(lowerLine string, terms []string) int {
	distinct, total := 0, 0
	for _, term := range terms {
		count := strings.Count(lowerLine, term)
		if count == 0 {
			continue
		}
		distinct++
		total += count
	}
	if distinct == 0 {
		return 0
	}
	score := distinct*distinctTermWeight + total
	if strings.HasPrefix(strings.TrimSpace(lowerLine), "#") {
		score += headingBonus
	}
	return score
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
