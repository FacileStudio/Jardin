package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const embedBatch = 32

// TestHybridBeatsLexical is the decision the whole retrieval stack rests on:
// whether dense retrieval and fusion earn the two services they cost. It runs
// only when OLLAMA_URL is set, so the ordinary suite never depends on a model
// being up.
func TestHybridBeatsLexical(t *testing.T) {
	base := os.Getenv("OLLAMA_URL")
	if base == "" {
		t.Skip("set OLLAMA_URL to measure dense and hybrid retrieval")
	}
	dir, ok := wikiDir()
	if !ok {
		t.Skip("no wiki on this machine")
	}
	cases := loadGolden(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	backend := NewOllama(base, embedModelName())
	model, err := backend.Model(ctx)
	if err != nil {
		t.Fatalf("backend unreachable: %v", err)
	}
	t.Logf("model %s (%d dims)", model.Name, model.Dims)

	store := indexWiki(ctx, t, dir, backend, model)
	report(t, "lexical", measureRanker(t, dir, cases, SearchChunks))
	report(t, "dense", measureRanker(t, dir, cases, denseRanker(ctx, t, backend, store)))
	report(t, "hybrid", measureRanker(t, dir, cases, hybridRanker(ctx, t, backend, store)))
}

func evalIndexDir(t *testing.T, model ModelID) string {
	t.Helper()
	base := os.Getenv("EMBED_INDEX_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "jardin-eval-index")
	}
	dir := filepath.Join(base, model.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func embedModelName() string {
	if name := os.Getenv("EMBED_MODEL"); name != "" {
		return name
	}
	return "bge-m3"
}

func indexWiki(ctx context.Context, t *testing.T, dir string, backend Backend, model ModelID) Store {
	t.Helper()
	store, err := OpenFlatStore(evalIndexDir(t, model), model)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := readDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	var chunks []Chunk
	for _, p := range pages {
		chunks = append(chunks, Chunks(p.path, p.body)...)
	}
	known := store.Hashes()
	var todo []Chunk
	for _, c := range chunks {
		if known[ChunkKey(c)] != ChunkHash(c) {
			todo = append(todo, c)
		}
	}
	started := time.Now()
	for start := 0; start < len(todo); start += embedBatch {
		end := min(start+embedBatch, len(todo))
		upsertBatch(ctx, t, backend, store, todo[start:end])
		t.Logf("embedded %d/%d chunks (%s elapsed)", end, len(todo), time.Since(started).Round(time.Second))
	}
	t.Logf("index ready: %d chunks total, %d embedded this run in %s",
		len(chunks), len(todo), time.Since(started).Round(time.Second))
	return store
}

func upsertBatch(ctx context.Context, t *testing.T, backend Backend, store Store, batch []Chunk) {
	t.Helper()
	texts := make([]string, len(batch))
	for i, c := range batch {
		texts[i] = c.Text()
	}
	vectors, err := backend.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	entries := make([]Entry, len(batch))
	for i, c := range batch {
		entries[i] = Entry{
			Key: ChunkKey(c), Path: c.Path, Heading: c.Heading,
			Line: c.Line, Hash: ChunkHash(c), Vector: vectors[i],
		}
	}
	if err := store.Upsert(entries); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func denseRanker(ctx context.Context, t *testing.T, backend Backend, store Store) ranker {
	return func(_, query string) ([]SearchResult, error) {
		vectors, err := backend.Embed(ctx, []string{query})
		if err != nil || len(vectors) == 0 {
			return nil, err
		}
		hits, err := store.Nearest(vectors[0], 50)
		if err != nil {
			return nil, err
		}
		return toResults(hits), nil
	}
}

func hybridRanker(ctx context.Context, t *testing.T, backend Backend, store Store) ranker {
	dense := denseRanker(ctx, t, backend, store)
	return func(dir, query string) ([]SearchResult, error) {
		lexical, err := SearchChunks(dir, query)
		if err != nil {
			return nil, err
		}
		vectors, err := dense(dir, query)
		if err != nil {
			return lexical, nil
		}
		return FuseRRF([][]SearchResult{trim(lexical, 50), trim(vectors, 50)}, 50), nil
	}
}

func toResults(scored []Scored) []SearchResult {
	results := make([]SearchResult, 0, len(scored))
	for _, s := range scored {
		results = append(results, SearchResult{Path: s.Path, Line: s.Line, Score: int(s.Score * 10000)})
	}
	return results
}

func trim(results []SearchResult, limit int) []SearchResult {
	if len(results) > limit {
		return results[:limit]
	}
	return results
}

func measureRanker(t *testing.T, dir string, cases []goldenCase, rank ranker) [2]float64 {
	t.Helper()
	recall, mrr := measure(t, dir, cases, rank)
	return [2]float64{recall, mrr}
}

func report(t *testing.T, label string, scores [2]float64) {
	t.Helper()
	t.Logf("%-8s recall@%d = %.3f   MRR = %.3f", label, evalK, scores[0], scores[1])
}
