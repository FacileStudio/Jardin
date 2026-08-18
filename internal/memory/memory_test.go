package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func wikiFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pages := map[string]string{
		"index.md":            "# Index\n- [tools/flow](tools/flow.md) - the trust store\n",
		"tools/flow.md":       "# Flow\n\n### The trust store is lazy\nA pin lands in the store only on the first trust.\nUnrelated line about postgres.\n",
		"bugs/unrelated.md":   "# Something else\nNothing to do with pinning at all.\n",
		"tools/half-match.md": "# Half\nOnly the word trust appears here, never the other one.\n",
	}
	for name, body := range pages {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestSearchIgnoresWordOrder is the bug that motivated ranking: the old literal
// match found "trust store" and missed "store trust".
func TestSearchIgnoresWordOrder(t *testing.T) {
	dir := wikiFixture(t)
	forward, err := Search(dir, "trust store")
	if err != nil {
		t.Fatal(err)
	}
	backward, err := Search(dir, "store trust")
	if err != nil {
		t.Fatal(err)
	}
	if len(forward) == 0 || len(backward) == 0 {
		t.Fatalf("both orders must match: forward=%d backward=%d", len(forward), len(backward))
	}
	if forward[0].Path != backward[0].Path {
		t.Fatalf("both orders must rank the same page first, got %q and %q", forward[0].Path, backward[0].Path)
	}
}

// TestSearchRequiresEveryTerm keeps a page that carries only one term out of
// the results, which is what made single-word matching so noisy.
func TestSearchRequiresEveryTerm(t *testing.T) {
	dir := wikiFixture(t)
	results, err := Search(dir, "trust store")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == filepath.Join("tools", "half-match.md") {
			t.Fatalf("a page missing one term must not match: %+v", r)
		}
	}
}

// TestSearchRanksTheDensestLineFirst proves the ordering is by match quality
// rather than by walk order.
func TestSearchRanksTheDensestLineFirst(t *testing.T) {
	dir := wikiFixture(t)
	results, err := Search(dir, "trust store")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("want several results, got %d", len(results))
	}
	if results[0].Score < results[len(results)-1].Score {
		t.Fatalf("results are not sorted by score: %+v", results)
	}
	if !contains(results[0].Content, "trust store") && !contains(results[0].Content, "store") {
		t.Fatalf("the top hit should carry both terms, got %q", results[0].Content)
	}
}

// TestSearchOnAnEmptyQueryFindsNothing keeps whitespace from matching the whole
// wiki.
func TestSearchOnAnEmptyQueryFindsNothing(t *testing.T) {
	dir := wikiFixture(t)
	results, err := Search(dir, "   ")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("want no results, got %d", len(results))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
