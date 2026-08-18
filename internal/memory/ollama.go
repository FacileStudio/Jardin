package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ollamaBatchSize   = 32
	ollamaTimeout     = 120 * time.Second
	ollamaBodyExcerpt = 256
	ollamaProbeText   = "probe"
)

// Ollama embeds text through a local Ollama instance. It is safe for
// concurrent use: the only mutable state is the cached model identity, which is
// resolved once behind a mutex so a reindex does not re-probe the server for
// every batch.
type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
	batch   int

	mu     sync.Mutex
	cached ModelID
}

// NewOllama builds a backend talking to the Ollama HTTP API at baseURL using
// the named model. The client carries a timeout because a model that hangs
// while loading would otherwise wedge a reindex until the process is killed.
func NewOllama(baseURL, model string) *Ollama {
	return &Ollama{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: ollamaTimeout},
		batch:   ollamaBatchSize,
	}
}

// Embed returns one vector per input text, in the same order. Texts are sent in
// capped batches so a full reindex never builds one enormous request body. A
// server returning a different number of embeddings than it was asked for is an
// error: padding the gap would silently misalign every vector after it.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([]Vector, error) {
	if len(texts) == 0 {
		return []Vector{}, nil
	}
	size := o.batch
	if size <= 0 {
		size = ollamaBatchSize
	}
	out := make([]Vector, 0, len(texts))
	for start := 0; start < len(texts); start += size {
		batch := texts[start:min(start+size, len(texts))]
		vectors, err := o.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		if len(vectors) != len(batch) {
			return nil, fmt.Errorf("ollama: model %q returned %d embeddings for %d texts",
				o.model, len(vectors), len(batch))
		}
		out = append(out, vectors...)
	}
	return out, nil
}

// Model reports the identity of the embedding model, caching it after the first
// success. Dimensions come from embedding a probe string, which is the only
// number guaranteed to match the vectors actually stored.
//
// A missing digest is retried on every call rather than cached. ModelID.Matches
// treats an empty digest as compatible, so caching one gap would leave the
// index permanently unable to notice a moved :latest tag — the drift the digest
// exists to catch, disarmed by one failed request. A fresh instance really does
// answer /api/tags with an empty list until a pull touches the model directory,
// so the gap is not hypothetical.
func (o *Ollama) Model(ctx context.Context) (ModelID, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cached.Dims > 0 {
		if o.cached.Digest == "" {
			o.cached.Digest = o.digest(ctx)
		}
		return o.cached, nil
	}
	vectors, err := o.Embed(ctx, []string{ollamaProbeText})
	if err != nil {
		return ModelID{}, err
	}
	if len(vectors[0]) == 0 {
		return ModelID{}, fmt.Errorf("ollama: model %q returned an empty probe embedding", o.model)
	}
	o.cached = ModelID{Name: o.model, Digest: o.digest(ctx), Dims: len(vectors[0])}
	return o.cached, nil
}

func (o *Ollama) embedBatch(ctx context.Context, texts []string) ([]Vector, error) {
	body := struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{Model: o.model, Input: texts}

	var reply struct {
		Embeddings []Vector `json:"embeddings"`
	}
	if err := o.post(ctx, "/api/embed", body, &reply); err != nil {
		return nil, err
	}
	return reply.Embeddings, nil
}

func (o *Ollama) digest(ctx context.Context) string {
	var reply struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := o.get(ctx, "/api/tags", &reply); err != nil {
		return ""
	}
	for _, m := range reply.Models {
		if m.Name == o.model || m.Name == o.model+":latest" {
			return m.Digest
		}
	}
	return ""
}

func (o *Ollama) post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("ollama: %s: encode request: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("ollama: %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return o.do(req, path, out)
}

func (o *Ollama) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("ollama: %s: build request: %w", path, err)
	}
	return o.do(req, path, out)
}

func (o *Ollama) do(req *http.Request, path string, out any) error {
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, ollamaBodyExcerpt))
		return fmt.Errorf("ollama: %s: status %d: %s", path, resp.StatusCode,
			strings.TrimSpace(string(excerpt)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("ollama: %s: decode response: %w", path, err)
	}
	return nil
}
