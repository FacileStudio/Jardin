package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

type doc struct {
	path    string
	page    string
	line    int
	display string
	body    string
	tokens  map[string]int
	length  int
	weight  float64
	date    string
	links   []string
}

// tokenize lowercases, folds accents away, and splits on anything that is not
// a letter or a digit. Folding matters because the wiki is largely French while
// the queries put to it rarely carry accents: without it, "chainage" cannot
// find "chaînage", and a whole language's worth of pages goes missing for
// anyone typing on a keyboard that makes accents inconvenient.
func tokenize(text string) []string {
	folded := strings.Map(foldAccent, strings.ToLower(text))
	return strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// foldAccent maps a lowercase accented Latin rune onto its unaccented form.
// It is a table rather than Unicode normalisation because golang.org/x/text is
// not a dependency of this binary and one search feature does not justify
// making it one; the ligatures it cannot express one-to-one are rare enough in
// the corpus to leave alone.
func foldAccent(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ã', 'ä', 'å':
		return 'a'
	case 'ç':
		return 'c'
	case 'è', 'é', 'ê', 'ë':
		return 'e'
	case 'ì', 'í', 'î', 'ï':
		return 'i'
	case 'ñ':
		return 'n'
	case 'ò', 'ó', 'ô', 'õ', 'ö':
		return 'o'
	case 'ù', 'ú', 'û', 'ü':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	}
	return r
}

// newDoc builds the whole-page unit the page-level search ranks. The body it is
// given still carries the frontmatter, so a `[[link]]` written there is already
// in reach, but a bare `related: [a, b]` list is not until it is parsed. A
// duplicate between the two costs nothing: the lift is a set membership test.
func newDoc(path, body string) doc {
	d := newUnit(path, path, 1, body)
	front, _ := frontmatter(body)
	d.links = append(d.links, splitRelated(front.related)...)
	return d
}

func newUnit(id, page string, line int, body string) doc {
	tokens := tokenize(body)
	counts := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		counts[tok]++
	}
	return doc{
		path:   id,
		page:   page,
		line:   line,
		body:   body,
		tokens: counts,
		length: len(tokens),
		weight: 1,
		links:  wikiLinks(body),
	}
}

// termFrequency counts a term in a document, treating the term as a prefix so
// "trust" also finds "trusted" and "sync" finds "syncing". It is the cheapest
// stemmer that helps more than it hurts on a wiki full of identifiers.
func (d doc) termFrequency(term string) int {
	total := 0
	for tok, n := range d.tokens {
		if strings.HasPrefix(tok, term) {
			total += n
		}
	}
	return total
}

type corpus struct {
	docs      []doc
	avgLength float64
}

func newCorpus(docs []doc) corpus {
	total := 0
	for _, d := range docs {
		total += d.length
	}
	avg := 1.0
	if len(docs) > 0 && total > 0 {
		avg = float64(total) / float64(len(docs))
	}
	return corpus{docs: docs, avgLength: avg}
}

func (c corpus) documentFrequency(term string) int {
	n := 0
	for _, d := range c.docs {
		if d.termFrequency(term) > 0 {
			n++
		}
	}
	return n
}

// inverseDocumentFrequency is what makes a paraphrased query work: a term in
// almost every page contributes nothing, while a rare identifier dominates.
func (c corpus) inverseDocumentFrequency(term string) float64 {
	n := float64(len(c.docs))
	df := float64(c.documentFrequency(term))
	return math.Log(1 + (n-df+0.5)/(df+0.5))
}

// score is BM25: saturating term frequency, length-normalised, IDF-weighted.
// Unlike a strict AND, a document missing some terms still ranks.
func (c corpus) score(d doc, weights []termWeight) float64 {
	total := 0.0
	for _, w := range weights {
		tf := float64(d.termFrequency(w.term))
		if tf == 0 {
			continue
		}
		norm := bm25K1 * (1 - bm25B + bm25B*float64(d.length)/c.avgLength)
		total += w.idf * (tf * (bm25K1 + 1)) / (tf + norm)
	}
	return total
}

// rank scores every document and fuses in the wiki-link signal. It returns the
// final scores and, for each, the ratio the link credit moved it by: the
// page-level search scores individual lines rather than the document, so it has
// to carry the same change one level down, and a ratio is the only form that
// survives the change of unit.
//
// A document with no lexical score at all keeps a ratio of 1. Its lines score
// zero whatever it is multiplied by, so the page-level search can reorder pages
// the query reached and never injects one it did not.
//
// The ratio is capped at 1+linkShare, and the cap is load-bearing rather than
// belt-and-braces. linkShare's ceiling is stated against a document score, and a
// line weight is not on that scale: a page scoring 0.01 that inherits 3.0 has a
// ratio of 301, which multiplied into its line weights buries the whole result
// list under one linked page. Measured, uncapped, that cost the page-level set
// recall 1.000 -> 0.979 and MRR 0.983 -> 0.962. A credit expressed in one unit
// does not convert into another for free.
func (c corpus) rank(weights []termWeight) ([]float64, []float64) {
	scores := make([]float64, len(c.docs))
	for i, d := range c.docs {
		scores[i] = c.score(d, weights) * d.weight
	}
	credits := linkCredits(c.docs, scores)
	ratios := make([]float64, len(c.docs))
	for i := range scores {
		ratios[i] = 1
		if scores[i] > 0 {
			ratios[i] = math.Min((scores[i]+credits[i])/scores[i], 1+linkShare)
		}
		scores[i] += credits[i]
	}
	return scores, ratios
}

// termWeight pairs a query term with its IDF. Weights are carried as an
// ordered slice, never a map: scoring sums floats, float addition is not
// associative, and Go randomises map iteration — so a map would make the same
// query return different scores on different runs.
type termWeight struct {
	term string
	idf  float64
}

func (c corpus) weights(terms []string) []termWeight {
	weights := make([]termWeight, 0, len(terms))
	for _, term := range terms {
		weights = append(weights, termWeight{term: term, idf: c.inverseDocumentFrequency(term)})
	}
	sort.Slice(weights, func(i, j int) bool { return weights[i].term < weights[j].term })
	return weights
}

// bestLines picks the lines of a document that carry the most query weight, so
// the caller sees why the page matched without opening it.
func bestLines(d doc, weights []termWeight, limit int, lift float64) []SearchResult {
	var hits []SearchResult
	for i, line := range strings.Split(d.body, "\n") {
		weight := lineWeight(line, weights)
		if weight == 0 {
			continue
		}
		hits = append(hits, SearchResult{
			Path:    d.path,
			Line:    i + 1,
			Content: strings.TrimSpace(line),
			Score:   int(weight * lift * 100),
		})
	}
	sortResults(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func lineWeight(line string, weights []termWeight) float64 {
	tokens := tokenize(line)
	total := 0.0
	for _, w := range weights {
		for _, tok := range tokens {
			if strings.HasPrefix(tok, w.term) {
				total += w.idf
				break
			}
		}
	}
	if total > 0 && strings.HasPrefix(strings.TrimSpace(line), "#") {
		total *= 1.15
	}
	return total
}
