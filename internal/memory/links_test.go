package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// pageRankOf is the position of a page in a chunk-level search counting each
// page once, where freshness_test.go's rankOf counts result rows and so returns
// a chunk's position. A link moves whole pages, and a page with three matching
// chunks would otherwise appear to have moved two places by holding still.
//
// It refuses to report a rank for a page that is not in the corpus. Returning 0
// there is what once let a link case pass vacuously on a renamed answer.
func pageRankOf(t *testing.T, dir, query, page string) int {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(page))); err != nil {
		t.Fatalf("asked for the rank of %s, which is not in the corpus", page)
	}
	results, err := SearchChunks(dir, query)
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	seen, position := map[string]bool{}, 0
	for _, r := range results {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		position++
		if r.Path == filepath.FromSlash(page) {
			return position
		}
	}
	return 0
}

const (
	answerPage = "conventions/hold-the-lock-for-the-shortest-span.md"
	answerBody = `---
title: Hold a lock for the shortest span you can
type: convention
---

### Narrow the critical section before you widen the pool
**Date**: 2026-03-04
**Source**: direct observation

Doing the slow part inside the critical section is what turns a short wait into a
queue. Compute first, take the lock, write, release.
`
)

// linkerBody is the page a query about pooled connections finds on its own
// words. Only its related: line connects it to the answer.
func linkerBody(related string) string {
	return `---
title: One endpoint held a pooled connection for a whole request
type: bug
` + related + `---

### A report endpoint checked out a pooled connection and kept it
**Date**: 2026-03-02
**Source**: direct observation

The report endpoint checked a pooled connection out at the start of the request
and held it while it rendered, so a pool sized for short queries was exhausted by
a handful of concurrent reports.
`
}

// noise gives the query somewhere else to go, so a rank in this corpus means
// something. Each page shares a word or two with the query and answers none of
// it.
func noise() map[string]string {
	pages := map[string]string{}
	for name, text := range map[string]string{
		"bugs/pool-metrics-lag-behind-the-pool.md":      "checkout counters are sampled, so a short exhaustion never appears",
		"bugs/request-timeout-shorter-than-query.md":    "the request gave up while the query kept running and held its row locks",
		"tools/pool-sizing-is-not-a-guess.md":           "a connection pool is sized from concurrency and service time, never from cores",
		"tools/trace-the-checkout-not-the-query.md":     "span the connection checkout, because the wait is where the request goes",
		"conventions/a-request-owns-one-connection.md":  "a handler that takes a second connection from the same pool can deadlock it",
		"conventions/render-outside-the-transaction.md": "rendering inside a transaction holds a connection for the whole response",
	} {
		pages[name] = "---\ntitle: " + name + "\ntype: note\n---\n\n### " + name + "\n**Date**: 2026-03-01\n\n" + text + "\n"
	}
	return pages
}

const poolQuery = "one report endpoint held a pooled connection for the whole request and starved everything else"

// TestLinkedPageGainsFromAStrongMatch is step 11's exit criterion, measured
// rather than observed. The same corpus is built twice and differs in one line:
// whether the page the query finds declares a link to the page that answers it.
//
// The answer page shares almost no vocabulary with the query, which is the point.
// A page reachable on its own words would show the ranker working and prove
// nothing about the link.
func TestLinkedPageGainsFromAStrongMatch(t *testing.T) {
	build := func(related string) string {
		pages := noise()
		pages[answerPage] = answerBody
		pages["bugs/pool-exhausted-by-one-endpoint.md"] = linkerBody(related)
		return tempCorpus(t, pages)
	}
	unlinked := build("")
	linked := build("related: [conventions/hold-the-lock-for-the-shortest-span]\n")

	linker := "bugs/pool-exhausted-by-one-endpoint.md"
	if rank := rankOf(t, linked, poolQuery, linker); rank != 1 {
		t.Fatalf("the linking page ranks %d, not 1: the case grades nothing unless the query finds it", rank)
	}

	before := rankOf(t, unlinked, poolQuery, answerPage)
	after := rankOf(t, linked, poolQuery, answerPage)
	t.Logf("answer page rank: %d without the link, %d with it", before, after)

	if after == 0 {
		t.Fatal("the answer page does not appear at all with the link in place")
	}
	if before != 0 && after >= before {
		t.Errorf("the answer page ranked %d without the link and %d with it: the link bought nothing", before, after)
	}
}

// TestLinkCreditNeverOvertakesItsSource holds linkShare's ceiling where it is
// stated. A credited page borrows against the score of the match that pointed at
// it, so it must land behind that match however weak its own words are.
func TestLinkCreditNeverOvertakesItsSource(t *testing.T) {
	pages := noise()
	pages[answerPage] = answerBody
	pages["bugs/pool-exhausted-by-one-endpoint.md"] = linkerBody(
		"related: [conventions/hold-the-lock-for-the-shortest-span]\n")
	dir := tempCorpus(t, pages)

	linker := rankOf(t, dir, poolQuery, "bugs/pool-exhausted-by-one-endpoint.md")
	answer := rankOf(t, dir, poolQuery, answerPage)
	if answer != 0 && answer <= linker {
		t.Errorf("the credited page ranks %d and the match that credited it %d", answer, linker)
	}
}

// TestDanglingLinkIsNotAnEdge covers the shapes the wiki writes that point at no
// page: a slug for a page not written yet, which the memory rules bless, and the
// two bracket false positives that are not links at all.
func TestDanglingLinkIsNotAnEdge(t *testing.T) {
	docs := []doc{
		newDoc("tools/a.md", "---\ntitle: A\n---\n\ntext"),
		newDoc("tools/b.md", "---\ntitle: B\n---\n\ntext"),
	}
	index := linkIndex(docs)
	for _, name := range []string{"not-written-yet", ":space:", "", "tools/c", "a.md"} {
		if page, ok := index[name]; ok {
			t.Errorf("%q resolved to %s, and nothing in this corpus is called that", name, page)
		}
	}
	if index["tools/a"] != "tools/a.md" || index["a"] != "tools/a.md" {
		t.Errorf("a real page stopped resolving: %v", index)
	}
}

// TestBracketsThatAreNotLinksEarnNothing covers the two false positives the
// live wiki writes for real: a POSIX character class in a grep, and bash test
// syntax. Both are asserted at the credit, not at the parser. wikiLinks reads
// bracket syntax and is entitled to return them; what must hold is that a name
// matching no page never becomes an edge, which is why resolution and not
// pattern-matching decides.
func TestBracketsThatAreNotLinksEarnNothing(t *testing.T) {
	docs := []doc{
		newDoc("tools/shell.md", "---\ntitle: Shell\n---\n\ngrep '[[:space:]]' f && [[ -f x ]] && [[real-page]]"),
		newDoc("tools/real-page.md", "---\ntitle: Real\n---\n\ntext"),
		newDoc("tools/space.md", "---\ntitle: Space\n---\n\ntext"),
	}
	credits := linkCredits(docs, []float64{3, 0, 0})
	if credits[1] == 0 {
		t.Error("the one real link in the line earned nothing")
	}
	if credits[2] != 0 {
		t.Errorf("[[:space:]] credited tools/space.md %.3f", credits[2])
	}
}

// TestSelfAndReciprocalLinksEarnNothing covers the two loops a one-hop credit
// could still inflate: a page that links to itself, and two strong matches that
// link to each other.
func TestSelfAndReciprocalLinksEarnNothing(t *testing.T) {
	docs := []doc{
		newDoc("a.md", "---\ntitle: A\nrelated: [a, b]\n---\n\nlock lock lock"),
		newDoc("b.md", "---\ntitle: B\nrelated: [a]\n---\n\nlock lock"),
	}
	scores := []float64{2, 1}
	for i, credit := range linkCredits(docs, scores) {
		if credit != 0 {
			t.Errorf("%s earned %.3f from a self-link or a reciprocal pair", docs[i].page, credit)
		}
	}
}

// TestInboundLinkCountEarnsNothing is the hub failure mode. A page every other
// page points at is the answer to no query on that ground alone; it earns credit
// only when a match this query actually found points at it.
func TestInboundLinkCountEarnsNothing(t *testing.T) {
	docs := []doc{
		newDoc("hub.md", "---\ntitle: Hub\n---\n\nunrelated"),
		newDoc("a.md", "---\ntitle: A\nrelated: [hub]\n---\n\nlock"),
		newDoc("b.md", "---\ntitle: B\nrelated: [hub]\n---\n\nlock"),
		newDoc("c.md", "---\ntitle: C\nrelated: [hub]\n---\n\nlock"),
		newDoc("d.md", "---\ntitle: D\n---\n\nlock"),
	}
	quiet := linkCredits(docs, []float64{0, 0, 0, 0, 3})
	if quiet[0] != 0 {
		t.Errorf("the hub earned %.3f from inbound links alone", quiet[0])
	}
	loud := linkCredits(docs, []float64{0, 3, 0, 0, 0})
	if loud[0] == 0 {
		t.Error("the hub earned nothing from a match that does point at it")
	}
}

// TestAmbiguousBasenameResolvesToNeither guards the call linkIndex makes where
// the live wiki forces one: projects/mycelium.md and tools/mycelium.md both exist and
// both are linked as [[mycelium]].
func TestAmbiguousBasenameResolvesToNeither(t *testing.T) {
	index := linkIndex([]doc{
		newDoc("projects/mycelium.md", "x"),
		newDoc("tools/mycelium.md", "x"),
		newDoc("tools/mise.md", "x"),
	})
	if page, ok := index["mycelium"]; ok {
		t.Errorf("[[mycelium]] resolved to %s, and it cannot be known which was meant", page)
	}
	if index["projects/mycelium"] != "projects/mycelium.md" {
		t.Error("the unambiguous spelling stopped resolving")
	}
	if index["mise"] != "tools/mise.md" {
		t.Error("an unambiguous basename stopped resolving")
	}
}

// TestRelatedSpellingsAllResolve covers the three forms the live wiki writes.
// Half its links sit in frontmatter, which Chunks strips before any chunk
// exists, so a form this misses is a set of edges that silently is not there.
func TestRelatedSpellingsAllResolve(t *testing.T) {
	for name, head := range map[string]string{
		"bare list":       "related: [tools/mise, conventions/x.md]",
		"bracket list":    "related: [[tools/mise]], [[conventions/x]]",
		"block sequence":  "related:\n  - \"[[tools/mise]]\"\n  - conventions/x\nconfidence: high",
		"empty list":      "related: []",
		"no related line": "confidence: high",
	} {
		got := relatedLinks(head)
		want := 2
		if name == "empty list" || name == "no related line" {
			want = 0
		}
		if len(got) != want {
			t.Errorf("%s: parsed %v, want %d targets", name, got, want)
		}
	}
}

// TestWikiLinksIgnoresQuotedNames is the fence bug: the link parser scanned the
// raw body, so a page documenting the syntax linked to whatever it quoted. The
// live wiki had exactly this on its retrieval-eval page.
func TestWikiLinksIgnoresQuotedNames(t *testing.T) {
	body := "real [[tools/mycelium]] link\n" +
		"quoted `[[projects/mycelium]]` span\n" +
		"```\n[[conventions/fenced]]\n```\n" +
		"after the fence [[tools/filet]]\n"

	got := wikiLinks(body)
	want := []string{"tools/mycelium", "tools/filet"}
	if len(got) != len(want) {
		t.Fatalf("wikiLinks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestWikiLinksSurvivesAnOddBacktick pins the failure direction. A malformed
// line must cost that line's links, never the rest of the page's.
func TestWikiLinksSurvivesAnOddBacktick(t *testing.T) {
	body := "stray ` backtick [[tools/lost]]\nnext line [[tools/kept]]\n"

	got := wikiLinks(body)
	if len(got) != 1 || got[0] != "tools/kept" {
		t.Fatalf("wikiLinks = %v, want [tools/kept]", got)
	}
}
