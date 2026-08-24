package consolidate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/memory"
)

// Similarity thresholds for the NOOP/CREATE/SUPERSEDE decision. Cosine over
// embeddings separates "related" from "same claim" well enough to earn two
// thresholds; the lexical fallback cannot, so offline SUPERSEDE uses the same
// bar as near-duplicate and leans entirely on the judge plus the date check
// for safety.
const (
	EmbedNearDuplicate   = 0.90
	EmbedSupersede       = 0.94
	LexicalNearDuplicate = 0.60
	weakMatchSimilarity  = 0.20
)

const (
	metricEmbeddings = "embeddings"
	metricLexical    = "lexical"
)

// Outcome is what consolidation decided to do with a candidate.
type Outcome int

const (
	OutcomeNoop Outcome = iota
	OutcomeCreate
	OutcomeSupersede
)

func (o Outcome) String() string {
	switch o {
	case OutcomeCreate:
		return "create"
	case OutcomeSupersede:
		return "supersede"
	default:
		return "noop"
	}
}

// Match points at the existing finding a candidate was consolidated against.
// Path is relative to the memory dir; Claim is the chunk text as indexed,
// struck-through spans already dropped.
type Match struct {
	Path       string
	Heading    string
	Line       int
	Claim      string
	Date       time.Time
	Similarity float64
}

// Decision is the consolidation outcome for one candidate. Match is nil below
// weakMatchSimilarity and always set for NOOP and SUPERSEDE.
type Decision struct {
	Outcome    Outcome
	Similarity float64
	Metric     string
	Match      *Match
}

// Deduper consolidates candidates against the wiki under MemoryPath. Backend
// may be nil or fail at call time; both degrade to lexical similarity so an
// offline machine still consolidates, just blunter.
type Deduper struct {
	MemoryPath string
	Backend    memory.Backend
	Judge      *Judge
}

func (d *Deduper) Decide(ctx context.Context, c Candidate) (Decision, error) {
	chunks, err := d.chunks()
	if err != nil {
		return Decision{}, err
	}
	if len(chunks) == 0 {
		return Decision{Outcome: OutcomeCreate}, nil
	}
	sims, metric := d.similarities(ctx, c.Text, chunks)
	best := argmax(sims)
	nearDup, supersede := thresholds(metric)

	dec := Decision{Similarity: sims[best], Metric: metric}
	if sims[best] < nearDup {
		dec.Outcome = OutcomeCreate
		if sims[best] >= weakMatchSimilarity {
			dec.Match = matchOf(chunks[best], sims[best])
		}
		return dec, nil
	}

	dec.Match = matchOf(chunks[best], sims[best])
	dec.Outcome = OutcomeNoop
	if d.Judge == nil {
		return dec, nil
	}
	judged := d.Judge.Compare(ctx, chunks[best].Text(), c.Text)
	if judged.Verdict != VerdictAccept || sims[best] < supersede {
		return dec, nil
	}
	episodeTime, ok := earliestTimestamp(c)
	if !ok || !chunks[best].Date().Before(episodeTime) {
		return dec, nil
	}
	dec.Outcome = OutcomeSupersede
	return dec, nil
}

func (d *Deduper) chunks() ([]memory.Chunk, error) {
	var chunks []memory.Chunk
	err := filepath.Walk(d.MemoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		rel, ok, err := d.pageRel(path)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		chunks = append(chunks, memory.Chunks(rel, string(data))...)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return chunks, err
}

func (d *Deduper) pageRel(path string) (string, bool, error) {
	if !strings.HasSuffix(path, ".md") {
		return "", false, nil
	}
	rel, err := filepath.Rel(d.MemoryPath, path)
	if err != nil {
		return "", false, err
	}
	rel = filepath.ToSlash(rel)
	if rel == "index.md" || rel == "log.md" {
		return "", false, nil
	}
	return rel, true, nil
}

func (d *Deduper) similarities(ctx context.Context, query string, chunks []memory.Chunk) ([]float64, string) {
	texts := make([]string, 0, len(chunks)+1)
	texts = append(texts, query)
	for _, c := range chunks {
		texts = append(texts, c.Text())
	}
	sims := make([]float64, len(chunks))
	if d.Backend != nil {
		vectors, err := d.Backend.Embed(ctx, texts)
		if err == nil && len(vectors) == len(texts) {
			for i := range chunks {
				sims[i] = memory.Cosine(vectors[0], vectors[i+1])
			}
			return sims, metricEmbeddings
		}
	}
	for i := range chunks {
		sims[i] = lexicalSimilarity(query, chunks[i].Text())
	}
	return sims, metricLexical
}

func thresholds(metric string) (nearDup, supersede float64) {
	if metric == metricEmbeddings {
		return EmbedNearDuplicate, EmbedSupersede
	}
	return LexicalNearDuplicate, LexicalNearDuplicate
}

func matchOf(c memory.Chunk, sim float64) *Match {
	return &Match{
		Path:       c.Path,
		Heading:    c.Heading,
		Line:       c.Line,
		Claim:      c.Text(),
		Date:       c.Date(),
		Similarity: sim,
	}
}

func argmax(sims []float64) int {
	best := 0
	for i, s := range sims {
		if s > sims[best] {
			best = i
		}
	}
	return best
}

// earliestTimestamp reads the oldest episode timestamp out of a candidate's
// refs, which heuristic.go writes as "<agent>@<RFC3339>". A candidate whose
// refs carry no parsable timestamp reports false, and the caller must treat
// SUPERSEDE as unavailable: striking a claim needs proof the correction is
// newer, not hope.
func earliestTimestamp(c Candidate) (time.Time, bool) {
	var best time.Time
	found := false
	for _, ref := range c.EpisodeRefs {
		at := strings.LastIndex(ref, "@")
		if at < 0 {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, ref[at+1:])
		if err != nil {
			continue
		}
		if !found || ts.Before(best) {
			best, found = ts, true
		}
	}
	return best, found
}

// lexicalSimilarity is the offline stand-in for embeddings: an overlap
// coefficient of the two texts' lowercase word tokens, intersection over the
// smaller token set. Blunt by design — it knows words, not meaning — which is
// exactly why its thresholds sit apart from the embedding ones.
func lexicalSimilarity(a, b string) float64 {
	at := tokenize(a)
	bt := tokenize(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	intersection := 0
	smaller := len(at)
	if len(bt) < smaller {
		smaller = len(bt)
	}
	for tok := range at {
		if bt[tok] {
			intersection++
		}
	}
	return float64(intersection) / float64(smaller)
}

func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	}) {
		if len(field) > 2 {
			out[field] = true
		}
	}
	return out
}
