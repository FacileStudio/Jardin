package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

type fakeOllama struct {
	mu       sync.Mutex
	batches  [][]string
	tagCalls int
	status   int
	surplus  int
	hold     chan struct{}
	started  chan struct{}
}

func (f *fakeOllama) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			f.mu.Lock()
			f.tagCalls++
			f.mu.Unlock()
			writeFakeJSON(w, map[string]any{"models": []map[string]string{
				{"name": "bge-m3:latest", "digest": "cafe1234"},
			}})
			return
		}
		f.serveEmbed(w, r)
	}
}

func (f *fakeOllama) serveEmbed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.batches = append(f.batches, body.Input)
	f.mu.Unlock()

	if f.started != nil {
		close(f.started)
		f.started = nil
	}
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-r.Context().Done():
			return
		}
	}
	if f.status != 0 {
		http.Error(w, `{"error":"model runner crashed"}`, f.status)
		return
	}
	writeFakeJSON(w, map[string]any{"embeddings": fakeVectors(body.Input, f.surplus)})
}

func (f *fakeOllama) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func fakeVectors(inputs []string, surplus int) [][]float32 {
	vectors := make([][]float32, 0, len(inputs)+surplus)
	for _, text := range inputs {
		vectors = append(vectors, []float32{textValue(text), 1, 2})
	}
	for i := 0; i < surplus; i++ {
		vectors = append(vectors, []float32{0, 1, 2})
	}
	return vectors
}

func textValue(text string) float32 {
	var sum float32
	for _, b := range []byte(text) {
		sum = sum*31 + float32(b)
	}
	return sum
}

func writeFakeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func startFake(t *testing.T, fake *fakeOllama) *Ollama {
	t.Helper()
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	return NewOllama(server.URL+"/", "bge-m3")
}

func labels(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "chunk-" + strconv.Itoa(i)
	}
	return out
}

// TestOllamaEmbedSplitsIntoCappedBatches keeps a full reindex from posting one
// enormous body: 33 texts against a cap of 32 must cross the wire as 32 + 1.
func TestOllamaEmbedSplitsIntoCappedBatches(t *testing.T) {
	fake := &fakeOllama{}
	client := startFake(t, fake)

	vectors, err := client.Embed(context.Background(), labels(33))
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 33 {
		t.Fatalf("got %d vectors, want 33", len(vectors))
	}
	if fake.calls() != 2 {
		t.Fatalf("got %d requests, want 2", fake.calls())
	}
	if len(fake.batches[0]) != 32 || len(fake.batches[1]) != 1 {
		t.Fatalf("got batch sizes %d and %d, want 32 and 1",
			len(fake.batches[0]), len(fake.batches[1]))
	}
}

// TestOllamaEmbedPreservesOrderAcrossBatches is the alignment gate. Vectors are
// keyed to their chunk by position alone, so a batch stitched back in the wrong
// order would silently attribute every embedding to the wrong page.
func TestOllamaEmbedPreservesOrderAcrossBatches(t *testing.T) {
	texts := labels(70)
	client := startFake(t, &fakeOllama{})

	vectors, err := client.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vectors), len(texts))
	}
	for i, text := range texts {
		if vectors[i][0] != textValue(text) {
			t.Fatalf("vector %d carries %v, want %v for %q",
				i, vectors[i][0], textValue(text), text)
		}
	}
}

// TestOllamaEmbedFailsOnServerError proves a non-200 surfaces as an error
// carrying the status, rather than an empty vector set the caller would index.
func TestOllamaEmbedFailsOnServerError(t *testing.T) {
	client := startFake(t, &fakeOllama{status: http.StatusInternalServerError})

	_, err := client.Embed(context.Background(), []string{"one"})
	if err == nil {
		t.Fatal("a 500 response produced no error")
	}
	if !contains(err.Error(), "500") || !contains(err.Error(), "model runner crashed") {
		t.Fatalf("error lost the status or body: %v", err)
	}
}

// TestOllamaEmbedFailsOnCountMismatch is the reason Embed never pads: a server
// answering with more vectors than texts must fail loudly, because trusting it
// would misalign every entry that follows.
func TestOllamaEmbedFailsOnCountMismatch(t *testing.T) {
	client := startFake(t, &fakeOllama{surplus: 1})

	if _, err := client.Embed(context.Background(), []string{"one", "two"}); err == nil {
		t.Fatal("a mismatched embedding count produced no error")
	}
}

// TestOllamaEmbedOnEmptyInputMakesNoRequest keeps an incremental reindex with
// nothing to do from touching the network at all.
func TestOllamaEmbedOnEmptyInputMakesNoRequest(t *testing.T) {
	fake := &fakeOllama{}
	client := startFake(t, fake)

	vectors, err := client.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 0 {
		t.Fatalf("got %d vectors, want 0", len(vectors))
	}
	if fake.calls() != 0 {
		t.Fatalf("got %d requests, want 0", fake.calls())
	}
}

// TestOllamaModelIsCachedAfterFirstCall stops every batch of a reindex from
// re-probing the server for an identity that cannot change mid-run.
func TestOllamaModelIsCachedAfterFirstCall(t *testing.T) {
	fake := &fakeOllama{}
	client := startFake(t, fake)

	first, err := client.Model(context.Background())
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	second, err := client.Model(context.Background())
	if err != nil {
		t.Fatalf("Model again: %v", err)
	}
	if first != second {
		t.Fatalf("identity changed between calls: %+v then %+v", first, second)
	}
	if first.Name != "bge-m3" || first.Digest != "cafe1234" || first.Dims != 3 {
		t.Fatalf("unexpected identity %+v", first)
	}
	if fake.calls() != 1 || fake.tagCalls != 1 {
		t.Fatalf("got %d probes and %d tag lookups, want 1 and 1", fake.calls(), fake.tagCalls)
	}
}

// TestOllamaEmbedHonoursContextCancellation proves a cancelled reindex stops at
// the in-flight request instead of running to the client timeout.
func TestOllamaEmbedHonoursContextCancellation(t *testing.T) {
	fake := &fakeOllama{hold: make(chan struct{}), started: make(chan struct{})}
	started := fake.started
	client := startFake(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Embed(ctx, []string{"one"})
		result <- err
	}()

	<-started
	cancel()
	err := <-result
	if err == nil {
		t.Fatal("a cancelled context produced no error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not wrap context.Canceled: %v", err)
	}
}

// TestModelRetriesAMissingDigest covers the drift this pin exists to catch: if
// the tags call fails once, caching the empty digest would leave the index
// unable to notice a moved :latest tag forever.
func TestModelRetriesAMissingDigest(t *testing.T) {
	tagCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			tagCalls++
			if tagCalls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"models":[{"name":"bge-m3:latest","digest":"abc123"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer server.Close()

	backend := NewOllama(server.URL, "bge-m3")
	first, err := backend.Model(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != "" {
		t.Fatalf("want an empty digest after the failed tags call, got %q", first.Digest)
	}
	second, err := backend.Model(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Digest != "abc123" {
		t.Fatalf("digest must be retried while empty, got %q", second.Digest)
	}
	if second.Dims != 3 {
		t.Fatalf("dims must stay cached, got %d", second.Dims)
	}
}
