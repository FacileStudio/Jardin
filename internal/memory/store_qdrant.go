package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	qdrantTimeout   = 30 * time.Second
	qdrantScrollLen = 256
	qdrantBatchLen  = 128
	qdrantBodyLimit = 4 << 20
	qdrantExcerpt   = 240
)

// QdrantStore is a Store backed by a Qdrant collection reached over its REST
// API. It holds no vectors locally: every method is a request, so a caller that
// cannot reach the server degrades to lexical search exactly as with a stale
// flat index.
type QdrantStore struct {
	baseURL    string
	collection string
	model      ModelID
	client     *http.Client
}

// OpenQdrantStore connects to a Qdrant instance and makes sure the collection
// exists with the geometry the model needs. An existing collection whose vector
// size differs from the model's is an error rather than a rebuild, because
// answering a query from another model's vectors is worse than answering none.
func OpenQdrantStore(baseURL, collection string, model ModelID) (*QdrantStore, error) {
	if model.Dims <= 0 {
		return nil, fmt.Errorf("qdrant: model %q declares no dimensions", model.Name)
	}
	if collection == "" {
		return nil, fmt.Errorf("qdrant: collection name is empty")
	}
	store := &QdrantStore{
		baseURL:    strings.TrimRight(baseURL, "/"),
		collection: collection,
		model:      model,
		client:     &http.Client{Timeout: qdrantTimeout},
	}
	if err := store.ensure(); err != nil {
		return nil, err
	}
	return store, nil
}

// Model reports the identity of the model whose vectors this store holds.
func (s *QdrantStore) Model() ModelID {
	return s.model
}

// Hashes scrolls the whole collection and returns the content hash of every
// indexed chunk by key, which is what lets an incremental index skip blocks
// that have not changed. An unreachable server yields what was read so far.
func (s *QdrantStore) Hashes() map[string]string {
	hashes := make(map[string]string)
	var offset json.RawMessage
	for {
		body := map[string]any{"limit": qdrantScrollLen, "with_payload": true, "with_vector": false}
		if len(offset) > 0 {
			body["offset"] = offset
		}
		var out qdrantScrollResponse
		if err := s.send(http.MethodPost, s.path("/points/scroll"), body, &out); err != nil {
			return hashes
		}
		for _, point := range out.Result.Points {
			if point.Payload.Key != "" {
				hashes[point.Payload.Key] = point.Payload.Hash
			}
		}
		offset = out.Result.NextPageOffset
		if len(out.Result.Points) == 0 || len(offset) == 0 || string(offset) == "null" {
			return hashes
		}
	}
}

// Upsert writes entries into the collection, replacing any point carrying the
// same key. Vectors of the wrong width are refused before the request, so a
// mismatched backend fails loudly instead of poisoning the index.
func (s *QdrantStore) Upsert(entries []Entry) error {
	points := make([]qdrantPoint, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Vector) != s.model.Dims {
			return fmt.Errorf("qdrant: entry %q has %d dims, model %q needs %d",
				entry.Key, len(entry.Vector), s.model.Name, s.model.Dims)
		}
		points = append(points, qdrantPoint{
			ID:     qdrantPointID(entry.Key),
			Vector: entry.Vector,
			Payload: qdrantPayload{
				Key: entry.Key, Path: entry.Path, Heading: entry.Heading,
				Line: entry.Line, Hash: entry.Hash,
			},
		})
	}
	for start := 0; start < len(points); start += qdrantBatchLen {
		end := min(start+qdrantBatchLen, len(points))
		body := map[string]any{"points": points[start:end]}
		if err := s.send(http.MethodPut, s.path("/points?wait=true"), body, nil); err != nil {
			return err
		}
	}
	return nil
}

// DeletePaths removes every point belonging to the given pages, matched on the
// path stored in the payload rather than on ids, so a page whose chunks moved
// still leaves nothing behind.
func (s *QdrantStore) DeletePaths(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	filter := map[string]any{
		"must": []any{map[string]any{"key": "path", "match": map[string]any{"any": paths}}},
	}
	body := map[string]any{"filter": filter}
	return s.send(http.MethodPost, s.path("/points/delete?wait=true"), body, nil)
}

// Nearest returns the closest chunks to the query, best first, ties broken on
// key so repeated runs rank identically. A failed request returns no results:
// retrieval is expected to degrade, not to panic.
func (s *QdrantStore) Nearest(query Vector, limit int) []Scored {
	if limit <= 0 || len(query) != s.model.Dims {
		return nil
	}
	body := map[string]any{"query": query, "limit": limit, "with_payload": true}
	var out qdrantQueryResponse
	if err := s.send(http.MethodPost, s.path("/points/query"), body, &out); err != nil {
		return nil
	}
	scored := make([]Scored, 0, len(out.Result.Points))
	for _, point := range out.Result.Points {
		scored = append(scored, Scored{
			Key:   point.Payload.Key,
			Path:  point.Payload.Path,
			Line:  point.Payload.Line,
			Score: point.Score,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Key < scored[j].Key
	})
	return scored
}

func (s *QdrantStore) ensure() error {
	status, payload, err := s.call(http.MethodGet, s.path(""), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		params := qdrantVectorParams{Size: s.model.Dims, Distance: "Cosine"}
		return s.send(http.MethodPut, s.path(""), map[string]any{"vectors": params}, nil)
	}
	if !qdrantOK(status) {
		return qdrantStatusErr(http.MethodGet, s.path(""), status, payload)
	}
	var info qdrantInfoResponse
	if err := json.Unmarshal(payload, &info); err != nil {
		return fmt.Errorf("qdrant: decode collection %q: %w", s.collection, err)
	}
	if size := info.Result.Config.Params.Vectors.Size; size != s.model.Dims {
		return fmt.Errorf("qdrant: collection %q holds %d-dim vectors, model %q needs %d",
			s.collection, size, s.model.Name, s.model.Dims)
	}
	return nil
}

func (s *QdrantStore) path(suffix string) string {
	return "/collections/" + url.PathEscape(s.collection) + suffix
}

func (s *QdrantStore) send(method, path string, body, out any) error {
	status, payload, err := s.call(method, path, body)
	if err != nil {
		return err
	}
	if !qdrantOK(status) {
		return qdrantStatusErr(method, path, status, payload)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("qdrant: decode %s %s: %w", method, path, err)
	}
	return nil
}

func (s *QdrantStore) call(method, path string, body any) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), qdrantTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("qdrant: encode %s %s: %w", method, path, err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, qdrantBodyLimit))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("qdrant: read %s %s: %w", method, path, err)
	}
	return resp.StatusCode, payload, nil
}

func qdrantOK(status int) bool {
	return status >= 200 && status < 300
}

func qdrantStatusErr(method, path string, status int, payload []byte) error {
	excerpt := strings.Join(strings.Fields(string(payload)), " ")
	if len(excerpt) > qdrantExcerpt {
		excerpt = excerpt[:qdrantExcerpt] + "..."
	}
	return fmt.Errorf("qdrant: %s %s: status %d: %s", method, path, status, excerpt)
}

func qdrantPointID(key string) uint64 {
	sum := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(sum[:8])
}

type qdrantVectorParams struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

type qdrantPayload struct {
	Key     string `json:"key"`
	Path    string `json:"path"`
	Heading string `json:"heading"`
	Line    int    `json:"line"`
	Hash    string `json:"hash"`
}

type qdrantPoint struct {
	ID      uint64        `json:"id"`
	Vector  Vector        `json:"vector"`
	Payload qdrantPayload `json:"payload"`
}

type qdrantInfoResponse struct {
	Result struct {
		Config struct {
			Params struct {
				Vectors qdrantVectorParams `json:"vectors"`
			} `json:"params"`
		} `json:"config"`
	} `json:"result"`
}

type qdrantQueryResponse struct {
	Result struct {
		Points []struct {
			Score   float64       `json:"score"`
			Payload qdrantPayload `json:"payload"`
		} `json:"points"`
	} `json:"result"`
}

type qdrantScrollResponse struct {
	Result struct {
		Points []struct {
			Payload qdrantPayload `json:"payload"`
		} `json:"points"`
		NextPageOffset json.RawMessage `json:"next_page_offset"`
	} `json:"result"`
}
