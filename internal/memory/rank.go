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
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func newDoc(path, body string) doc {
	return newUnit(path, path, 1, body)
}

func newUnit(id, page string, line int, body string) doc {
	tokens := tokenize(body)
	counts := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		counts[tok]++
	}
	return doc{path: id, page: page, line: line, body: body, tokens: counts, length: len(tokens)}
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
func bestLines(d doc, weights []termWeight, limit int) []SearchResult {
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
			Score:   int(weight * 100),
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
