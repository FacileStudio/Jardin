package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const fixtureLinkFile = "testdata/golden-links.json"

// linkSignalLanded is false until a link-aware ranker ships. While it is false
// every answer page must sit outside the top five, which is what makes this set
// an instrument rather than another saturated case list. The change that adds
// the signal flips this and raises linkRecallFloor to what it measures.
//
// This is a switch rather than an assertion to invert, because the obvious
// shape — assert the answer stays outside the top five — fails the moment the
// feature works and blames the fixture for it.
const linkSignalLanded = false

// linkRecallFloor is recall@evalK on the link set. It is 0.000 today by
// construction: the answers are unreachable without following a link.
const linkRecallFloor = 0.0

// linkCase is a query whose answer is reachable only through the link graph.
// Linker is the page lexical scoring does find; Expect is the page it links to.
type linkCase struct {
	Query  string   `json:"query"`
	Expect []string `json:"expect"`
	Linker string   `json:"linker"`
}

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// loadLinkSet reads the link set and fails if it names a page that is not
// committed. TestFixtureRetrieval's set has this guard and this one did not,
// which let a case rot silently: rankOfFirstExpected returns 0 for a page that
// does not exist, so a renamed answer satisfied "outside the top five" while
// the stale slug still sat in the linker's related: line. The case became
// permanently unwinnable and read as the link ranker failing to improve.
func loadLinkSet(t *testing.T) []linkCase {
	t.Helper()
	data, err := os.ReadFile(fixtureLinkFile)
	if err != nil {
		t.Fatalf("link set unreadable: %v", err)
	}
	var cases []linkCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("link set is not valid JSON: %v", err)
	}
	if len(cases) < 5 {
		t.Fatalf("link set has %d cases, want at least 5", len(cases))
	}
	for _, c := range cases {
		for _, page := range append([]string{c.Linker}, c.Expect...) {
			if _, err := os.Stat(filepath.Join(fixtureCorpusDir, page)); err != nil {
				t.Fatalf("link set names a page that is not committed: %q (query %q)", page, c.Query)
			}
		}
	}
	return cases
}

// outsideFences drops fenced code blocks. Documentation is not a link: a page
// showing a related: example or quoting [[a-slug]] in a fence would otherwise
// register as linking there, and a case using it as its linker would satisfy the
// link assertion with no link in existence.
func outsideFences(body string) string {
	var kept []string
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// linkTargets returns the page names a page links to, normalised to a bare slug
// so the spellings in use all compare equal: [[slug]], [dir/slug.md], a bare
// [slug, other] list and a YAML block sequence.
//
// Fences are stripped first, which is what handles `[[:space:]]` and bash's
// `[[ ]]` in this corpus. The colon rule below is a second line of defence, not
// the first: it only ever caught the POSIX class by luck.
func linkTargets(t *testing.T, page string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureCorpusDir, page))
	if err != nil {
		t.Fatalf("linker page unreadable: %v", err)
	}
	body := outsideFences(string(data))
	out := map[string]bool{}
	for _, m := range wikiLinkPattern.FindAllStringSubmatch(body, -1) {
		if target := normaliseLink(m[1]); target != "" {
			out[target] = true
		}
	}
	collectRelated(body, out)
	return out
}

// collectRelated reads inline `related: [a, b]` lists and the YAML block
// sequence form, where each target is a `- item` line under the key.
func collectRelated(body string, out map[string]bool) {
	inBlock := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if item, ok := strings.CutPrefix(trimmed, "- "); inBlock && ok {
			if target := normaliseLink(item); target != "" {
				out[target] = true
			}
			continue
		}
		inBlock = false
		rest, found := strings.CutPrefix(trimmed, "related:")
		if !found {
			continue
		}
		if strings.TrimSpace(rest) == "" {
			inBlock = true
			continue
		}
		for _, raw := range strings.Split(strings.Trim(strings.TrimSpace(rest), "[]"), ",") {
			if target := normaliseLink(raw); target != "" {
				out[target] = true
			}
		}
	}
}

// normaliseLink reduces a link target to a bare slug, or returns empty for
// something that is not a link at all.
func normaliseLink(raw string) string {
	target := strings.Trim(strings.TrimSpace(raw), "[]\"' ")
	if target == "" || strings.Contains(target, ":") {
		return ""
	}
	target = strings.TrimSuffix(target, ".md")
	return filepath.Base(target)
}

// TestLinkCasesAreWellFormed asserts what must hold whatever the ranker does:
// both pages exist, the linker is inside the top five, and the linker links to
// the answer. None of this changes when a link signal ships.
func TestLinkCasesAreWellFormed(t *testing.T) {
	for _, c := range loadLinkSet(t) {
		rank := rankOfFirstExpected(t, fixtureCorpusDir, EvalCase{Query: c.Query, Expect: []string{c.Linker}})
		if rank == 0 || rank > evalK {
			t.Errorf("query %q: linker %s ranks %d, must be inside the top %d or the case tests nothing",
				c.Query, c.Linker, rank, evalK)
		}
		targets := linkTargets(t, c.Linker)
		for _, want := range c.Expect {
			if !targets[strings.TrimSuffix(filepath.Base(want), ".md")] {
				t.Errorf("query %q: %s does not link to %s, so nothing connects them", c.Query, c.Linker, want)
			}
		}
	}
}

// TestLinkRetrieval is where step 11 is graded. Every other set in this package
// sits at recall 1.000 and can only show a regression; this one starts at 0.000
// and can only show an improvement.
func TestLinkRetrieval(t *testing.T) {
	cases := loadLinkSet(t)
	scored := make([]EvalCase, 0, len(cases))
	for _, c := range cases {
		scored = append(scored, EvalCase{Query: c.Query, Expect: c.Expect})
	}
	recall, mrr := scoreSet(t, fixtureCorpusDir, scored)
	t.Logf("link recall@%d = %.3f (%d cases)   MRR = %.3f", evalK, recall, len(cases), mrr)

	if recall < linkRecallFloor {
		t.Errorf("link recall@%d dropped to %.3f, floor is %.2f", evalK, recall, linkRecallFloor)
	}
	if !linkSignalLanded && recall > 0 {
		t.Errorf("link recall is %.3f with no link signal shipped: a case is answerable lexically and grades nothing",
			recall)
	}
}

// TestCorpusBasenamesAreUnique guards the assumption normaliseLink rests on.
// Link targets are written several ways, so they are compared as bare slugs;
// that only distinguishes pages while no two share a basename.
//
// The live wiki already violates this: projects/mycelium.md and tools/mycelium.md
// both exist and are linked as both [[mycelium]] and [[tools/mycelium]]. A
// production resolver therefore cannot use basenames. This guards the fixture so
// the contract above stays meaningful, and is not a model for step 11.
func TestCorpusBasenamesAreUnique(t *testing.T) {
	seen := map[string]string{}
	err := filepath.Walk(fixtureCorpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		slug := strings.TrimSuffix(filepath.Base(path), ".md")
		if first, dup := seen[slug]; dup {
			t.Errorf("%s and %s share the basename %q, which normaliseLink cannot tell apart", first, path, slug)
		}
		seen[slug] = path
		return nil
	})
	if err != nil {
		t.Fatalf("walking the corpus: %v", err)
	}
}
