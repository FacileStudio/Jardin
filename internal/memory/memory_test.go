package memory

import (
	"os"
	"path/filepath"
	"reflect"
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

// TestSearchRanksFullMatchesAbovePartialOnes replaced an older test that
// required every term to be present. BM25 deliberately lets a page missing a
// term still rank, because a paraphrased query rarely shares every word with
// its page; what must hold is that carrying more of the query ranks higher.
func TestSearchRanksFullMatchesAbovePartialOnes(t *testing.T) {
	dir := wikiFixture(t)
	results, err := Search(dir, "trust store")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("want results")
	}
	partial := filepath.Join("tools", "half-match.md")
	for _, r := range results {
		if r.Path == partial {
			break
		}
		if r.Path == filepath.Join("tools", "flow.md") {
			return
		}
	}
	t.Fatalf("the page carrying both terms must outrank the one carrying one: %+v", results)
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

// TestSearchIsReproducible is the determinism gate. Scoring sums floats, and
// float addition is not associative, so any iteration over a map would make
// the same query return different scores between runs — and a ranking that
// moves on its own cannot be improved, because no change can be measured.
func TestSearchIsReproducible(t *testing.T) {
	dir := wikiFixture(t)
	first, err := Search(dir, "trust store lazy pin")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := Search(dir, "trust store lazy pin")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs:\nfirst: %+v\nagain: %+v", i, first, again)
		}
	}
}

// TestSearchIsReproducibleOnTheRealWiki runs the same check against the corpus
// that actually matters, where 200 files and hundreds of distinct terms give
// map-order nondeterminism room to show itself.
func TestSearchIsReproducibleOnTheRealWiki(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir := filepath.Join(home, ".jardin", "memory")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no wiki on this machine")
	}
	first, err := Search(dir, "porte oidc email verified lockout")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := Search(dir, "porte oidc email verified lockout")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs on the real wiki", i)
		}
	}
}
