package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	oauthBeta    = "oauth-2025-04-20"
	oauthTimeout = 5 * time.Second

	// OAuthCacheTTL keeps the endpoint at arm's length: it rate-limits hard
	// (HTTP 429), so a fresh cache answers without a request at all.
	OAuthCacheTTL = 5 * time.Minute
)

// oauthEndpoint is a var only so tests can point it at an httptest server. No
// refresh flow exists on purpose: exchanging a refresh token would rotate the
// credential Claude Code itself depends on.
var oauthEndpoint = "https://api.anthropic.com/api/oauth/usage"

// oauthBucket keeps utilization a pointer so an unrelated nested object in the
// payload is skipped instead of becoming a phantom window sitting at 0%.
type oauthBucket struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    int64    `json:"resets_at"`
}

type oauthCache struct {
	FetchedAt time.Time `json:"fetched_at"`
	Snapshot  Snapshot  `json:"snapshot"`
}

func oauthCachePath(dataDir string) string {
	return filepath.Join(dataDir, "usage", ".oauth-cache.json")
}

func readOAuthCache(dataDir string) (oauthCache, bool) {
	data, err := os.ReadFile(oauthCachePath(dataDir))
	if err != nil {
		return oauthCache{}, false
	}
	var cache oauthCache
	if json.Unmarshal(data, &cache) != nil || cache.FetchedAt.IsZero() {
		return oauthCache{}, false
	}
	return cache, true
}

func writeOAuthCache(dataDir string, s Snapshot) {
	data, err := json.MarshalIndent(oauthCache{FetchedAt: time.Now().UTC(), Snapshot: s}, "", "  ")
	if err != nil {
		return
	}
	writeAtomic(oauthCachePath(dataDir), data)
}

// FetchOAuth reads the subscription limits straight from Anthropic's OAuth
// usage endpoint. The endpoint is aggressively rate-limited, so a cache younger
// than OAuthCacheTTL short-circuits the request and any failure — 429 included
// — falls back to the last good response rather than erroring.
func FetchOAuth(ctx context.Context, dataDir, token string) (Snapshot, error) {
	if cache, ok := readOAuthCache(dataDir); ok && time.Since(cache.FetchedAt) < OAuthCacheTTL {
		return cache.Snapshot, nil
	}
	snapshot, err := fetchOAuth(ctx, token)
	if err != nil {
		if cache, ok := readOAuthCache(dataDir); ok && !errors.Is(err, ErrTokenRejected) {
			return cache.Snapshot, nil
		}
		return Snapshot{}, err
	}
	writeOAuthCache(dataDir, snapshot)
	return snapshot, nil
}

// fetchOAuth pulls a usage snapshot from the OAuth status endpoint. The
// response body is never surfaced or logged: it is the one place an echoed
// credential could leak into a terminal or a log file.
func fetchOAuth(ctx context.Context, token string) (Snapshot, error) {
	if token == "" {
		return Snapshot{}, errors.New("no usage token — run `mycelium usage login` with a token from `claude setup-token`")
	}
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthEndpoint, nil)
	if err != nil {
		return Snapshot{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", oauthBeta)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Snapshot{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Snapshot{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Snapshot{}, ErrTokenRejected
	}
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("oauth usage: %s", resp.Status)
	}
	return parseOAuth(body)
}

// parseOAuth converts the endpoint's payload: buckets sit at the top level and
// carry utilization as a 0..1 fraction, not a percentage.
func parseOAuth(body []byte) (Snapshot, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("decode oauth usage: %w", err)
	}
	snapshot := Snapshot{Source: SourceOAuth, UpdatedAt: time.Now().UTC().Truncate(time.Second), Windows: []Window{}}
	for key, value := range raw {
		var bucket oauthBucket
		if json.Unmarshal(value, &bucket) != nil || bucket.Utilization == nil {
			continue
		}
		snapshot.Windows = append(snapshot.Windows, Window{
			Key:            key,
			Label:          Label(key),
			UsedPercentage: *bucket.Utilization * 100,
			ResetsAt:       epochToTime(bucket.ResetsAt),
		})
	}
	if len(snapshot.Windows) == 0 {
		return snapshot, ErrNoRateLimits
	}
	sortWindows(snapshot.Windows)
	return snapshot, nil
}
