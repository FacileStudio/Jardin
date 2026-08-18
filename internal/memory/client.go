package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	remoteSearchPath  = "/api/memory/search"
	remoteBodyExcerpt = 256
	remoteScoreScale  = 1e6
)

// RemoteSearch is one query put to a Jardin server's memory search endpoint.
// It is a struct rather than a parameter list because every field is optional
// in a different way: an empty SpaceID searches the common tree, an empty Token
// is an unauthenticated call, and a zero Limit lets the server pick.
type RemoteSearch struct {
	BaseURL string
	Token   string
	SpaceID string
	Query   string
	Limit   int
}

// SearchRemote runs a query against the server's hybrid search and reports
// whether the server answered in a degraded state, meaning the vector half did
// not contribute and the ranking is lexical-only. Callers are expected to treat
// any error as "fall back to the local index" rather than as a failed search:
// this function never being reachable is a normal operating condition.
func SearchRemote(ctx context.Context, req RemoteSearch) ([]SearchResult, bool, error) {
	httpReq, err := buildRemoteSearch(ctx, req)
	if err != nil {
		return nil, false, err
	}
	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, false, fmt.Errorf("memory search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, remoteBodyExcerpt))
		return nil, false, fmt.Errorf("memory search: status %d: %s", resp.StatusCode,
			strings.TrimSpace(string(excerpt)))
	}

	var reply remoteReply
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return nil, false, fmt.Errorf("memory search: decode response: %w", err)
	}
	return reply.results(), reply.Degraded, nil
}

type remoteReply struct {
	Results []struct {
		Path    string  `json:"path"`
		Heading string  `json:"heading"`
		Line    int     `json:"line"`
		Score   float64 `json:"score"`
		Excerpt string  `json:"excerpt"`
	} `json:"results"`
	Degraded bool `json:"degraded"`
}

// results flattens the wire shape onto the same type the local index returns,
// so a caller prints one kind of result whichever half answered. Fused ranks
// are small floats and SearchResult scores are integers, hence the scale: the
// number is only ever compared against its siblings from the same query.
func (r remoteReply) results() []SearchResult {
	out := make([]SearchResult, 0, len(r.Results))
	for _, hit := range r.Results {
		out = append(out, SearchResult{
			Path:    hit.Path,
			Line:    hit.Line,
			Content: hit.Excerpt,
			Score:   int(hit.Score * remoteScoreScale),
		})
	}
	return out
}

func buildRemoteSearch(ctx context.Context, req RemoteSearch) (*http.Request, error) {
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if base == "" {
		return nil, errors.New("memory search: no server configured")
	}
	payload, err := json.Marshal(struct {
		Query   string `json:"query"`
		SpaceID string `json:"space_id"`
		Limit   int    `json:"limit"`
	}{Query: req.Query, SpaceID: req.SpaceID, Limit: req.Limit})
	if err != nil {
		return nil, fmt.Errorf("memory search: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+remoteSearchPath,
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("memory search: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.Token)
	}
	return httpReq, nil
}
