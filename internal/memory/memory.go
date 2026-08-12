package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SearchResult is one matching line from a memory file.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// Search returns every line under memoryPath that contains the query,
// case-insensitively, as SearchResults with paths relative to memoryPath.
func Search(memoryPath, query string) ([]SearchResult, error) {
	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	var results []SearchResult
	err = filepath.Walk(memoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if pattern.MatchString(line) {
				rel, _ := filepath.Rel(memoryPath, path)
				results = append(results, SearchResult{
					Path:    rel,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})
	return results, err
}

// ReadIndex returns the memory index.md file.
func ReadIndex(memoryPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(memoryPath, "index.md"))
	if err != nil {
		return "", fmt.Errorf("memory index not found: %w", err)
	}
	return string(data), nil
}
