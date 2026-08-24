package memory

import (
	"path"
	"sort"
	"strings"
)

const (
	// linkShare is the fraction of a strong match's score that a page it links to
	// inherits. Additive, and deliberately not the multiplier B3 used for
	// recency: a multiplier scales what a page already earned from the query's
	// own words, and the pages worth following a link to are exactly the ones
	// that earned almost nothing. Fifteen percent of nearly zero is nearly zero,
	// which is a signal that cannot fire where it is needed. Measured: as a 1.15
	// multiplier this moved the link set from MRR 0.027 to 0.031 and left recall
	// at 0.000.
	//
	// Thirty percent is the ceiling and it is the reason this stays safe. A
	// linked page can reach at most 0.30 of the score of the match that pointed
	// at it, so it can never overtake that match, and it never displaces a page
	// scoring more than 30% of the best hit on the query's own words. The
	// borrowed credit is capped by the lender, not by the borrower.
	linkShare = 0.30

	// linkSeeds is how many top-ranked pages get to vote. The signal has to stay
	// query-conditioned to be worth anything: with three seeds and the three to
	// eight links a page carries here, a query credits on the order of ten pages
	// out of seventy. Widen it and the neighbourhood becomes the corpus, at which
	// point everything is credited and the term says nothing.
	linkSeeds = 3
)

// wikiLinks pulls the [[page-name]] references out of a body. It returns the
// raw names lowercased with any ".md", "#anchor" or "|alias" removed; resolving
// them to pages is linkIndex's job, because a name that matches nothing must
// stay a name and never become an edge.
//
// A run that never closes ends the scan rather than swallowing the rest of the
// body, and a name spanning a newline is dropped: both shapes come from prose
// that happens to contain brackets, not from a link.
func wikiLinks(body string) []string {
	var links []string
	for i := 0; i < len(body); {
		open := strings.Index(body[i:], "[[")
		if open < 0 {
			return links
		}
		i += open + 2
		end := strings.Index(body[i:], "]]")
		if end < 0 {
			return links
		}
		name := body[i : i+end]
		i += end + 2
		if cut := strings.IndexAny(name, "|#"); cut >= 0 {
			name = name[:cut]
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, "\n") {
			continue
		}
		links = append(links, strings.ToLower(strings.TrimSuffix(name, ".md")))
	}
	return links
}

// relatedLinks reads the pages a frontmatter block declares under `related:`.
//
// This is half the graph, not a nicety. In the live wiki 39 of the 79 wiki-link
// spellings sit in frontmatter and 40 in bodies, and `Chunks` strips the
// frontmatter before any chunk is built — so a ranker reading only chunk bodies
// would see half the edges and call it the graph.
//
// Three spellings are in use and all three resolve: the inline
// `related: [a, b]` list, the same line written `related: [[a]], [[b]]`, and the
// YAML block sequence of `- "[[a]]"` items under a bare `related:`. A key other
// than `related` ends the block, so a `sources:` list underneath is not read as
// more links.
func relatedLinks(head string) []string {
	var links []string
	inBlock := false
	for _, line := range strings.Split(head, "\n") {
		trimmed := strings.TrimSpace(line)
		if item, ok := strings.CutPrefix(trimmed, "- "); inBlock && ok {
			links = appendLink(links, item)
			continue
		}
		inBlock = false
		rest, found := strings.CutPrefix(trimmed, "related:")
		if !found {
			continue
		}
		if rest = strings.TrimSpace(rest); rest == "" {
			inBlock = true
			continue
		}
		for _, raw := range strings.Split(strings.Trim(rest, "[]"), ",") {
			links = appendLink(links, raw)
		}
	}
	return links
}

// splitRelated reverses the comma join Chunk.Related carries.
func splitRelated(related string) []string {
	if related == "" {
		return nil
	}
	return strings.Split(related, ",")
}

// appendLink normalises one written link target and drops anything that is not
// one. A name carrying a colon is a YAML key or a URL, never a page.
func appendLink(links []string, raw string) []string {
	name := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), "[]\"'"))
	if name == "" || strings.ContainsAny(name, ":\n") {
		return links
	}
	return append(links, strings.ToLower(strings.TrimSuffix(name, ".md")))
}

// linkIndex maps a link name onto the page it points at. Both shapes the wiki
// writes resolve: the full "conventions/porte-password-contract" and the bare
// "porte-password-contract".
//
// A bare name carried by two pages in different directories resolves to
// neither. The corpus has one such pair, tools/mycelium.md and projects/mycelium.md,
// and guessing between them would hand six links to the wrong page — a wrong
// edge is worse than no edge, because it lifts a page the author never pointed
// at. A dangling name simply stays out of the map: the wiki rules bless
// [[name]] for a page not written yet, so it marks intent and must not become a
// panic, an edge, or anything the ranker counts.
func linkIndex(docs []doc) map[string]string {
	full := map[string]string{}
	base := map[string]string{}
	clash := map[string]bool{}
	for _, d := range docs {
		slug := strings.ToLower(strings.TrimSuffix(strings.ReplaceAll(d.page, "\\", "/"), ".md"))
		if _, seen := full[slug]; seen {
			continue
		}
		full[slug] = d.page
		short := path.Base(slug)
		if short == slug {
			continue
		}
		if prev, seen := base[short]; seen && prev != d.page {
			clash[short] = true
			continue
		}
		base[short] = d.page
	}
	for short, page := range base {
		if clash[short] {
			continue
		}
		if _, taken := full[short]; !taken {
			full[short] = page
		}
	}
	return full
}

// linkCredits returns the score every document inherits from the link graph:
// linkShare of the strongest match that points at its page, and zero for a page
// nothing points at.
//
// The signal is directional and query-conditioned, which is the whole design.
// Counting how many links a page receives would make index.md and every hub the
// answer to every query; here nothing is scored for being popular, only for
// being pointed at by a chunk that already answered this question. A page with
// forty inbound links and no strong match pointing at it today earns nothing.
//
// One hop, one pass. Seventy pages do not need PageRank, and an iterative walk
// would turn two pages that link to each other into a feedback loop. A seed
// cannot credit another seed and a page cannot credit itself, so a reciprocal
// pair and a self-link both contribute nothing.
//
// Every chunk of a credited page gets the same term, which leaves that page's
// own best-matching chunk in front of its siblings: the credit is constant
// across them, so their relative order is still the query's to decide.
// Retrieval stays chunk-level and no page is promoted whole.
func linkCredits(docs []doc, scores []float64) []float64 {
	credits := make([]float64, len(docs))
	seeds := seedDocs(docs, scores)
	if len(seeds) == 0 {
		return credits
	}
	isSeed := map[string]bool{}
	for _, i := range seeds {
		isSeed[docs[i].page] = true
	}
	index := linkIndex(docs)
	best := map[string]float64{}
	for _, i := range seeds {
		for _, name := range docs[i].links {
			page, ok := index[name]
			if !ok || page == docs[i].page || isSeed[page] {
				continue
			}
			if earned := linkShare * scores[i]; earned > best[page] {
				best[page] = earned
			}
		}
	}
	for i := range docs {
		credits[i] = best[docs[i].page]
	}
	return credits
}

// seedDocs picks the best-scoring document of each of the top linkSeeds pages.
// Ties break on path and line rather than on slice order so two runs of one
// query always choose the same seeds.
func seedDocs(docs []doc, scores []float64) []int {
	order := make([]int, 0, len(docs))
	for i, s := range scores {
		if s > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if scores[i] != scores[j] {
			return scores[i] > scores[j]
		}
		if docs[i].page != docs[j].page {
			return docs[i].page < docs[j].page
		}
		return docs[i].line < docs[j].line
	})
	var seeds []int
	seen := map[string]bool{}
	for _, i := range order {
		if seen[docs[i].page] {
			continue
		}
		seen[docs[i].page] = true
		seeds = append(seeds, i)
		if len(seeds) == linkSeeds {
			break
		}
	}
	return seeds
}
