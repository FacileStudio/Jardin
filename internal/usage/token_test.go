package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeToken = "sk-ant-oat01-NEVER-LEAK-THIS-VALUE"

func TestResolveTokenPrefersEnv(t *testing.T) {
	t.Setenv(TokenEnv, "  from-env  ")
	t.Setenv(TokenEnvAlt, "from-alt")
	if got := ResolveToken("from-config"); got != "from-env" {
		t.Fatalf("got %q", got)
	}

	t.Setenv(TokenEnv, "")
	if got := ResolveToken("from-config"); got != "from-alt" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTokenFallsBackToConfig(t *testing.T) {
	t.Setenv(TokenEnv, "")
	t.Setenv(TokenEnvAlt, "")
	t.Setenv("PATH", t.TempDir())
	if got := ResolveToken(" from-config "); got != "from-config" {
		t.Fatalf("got %q", got)
	}
	if got := ResolveToken(""); got != "" {
		t.Fatalf("a missing token must be empty, not an error: %q", got)
	}
}

// TestTokenNeverLeaks is the invariant that matters: only percentages and reset
// timestamps may cross the wire or hit disk.
func TestTokenNeverLeaks(t *testing.T) {
	var sawHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHeader = r.Header.Get("Authorization")
		if r.Header.Get("anthropic-beta") != oauthBeta {
			t.Errorf("missing beta header")
		}
		w.Write([]byte(`{"five_hour":{"utilization":0.684,"resets_at":1765089001}}`))
	}))
	defer srv.Close()

	original := oauthEndpoint
	oauthEndpoint = srv.URL
	defer func() { oauthEndpoint = original }()

	dir := t.TempDir()
	snapshot, err := FetchOAuth(context.Background(), dir, fakeToken)
	if err != nil {
		t.Fatal(err)
	}
	if sawHeader != "Bearer "+fakeToken {
		t.Fatalf("authorization header wrong: %q", sawHeader)
	}
	if snapshot.Windows[0].UsedPercentage < 68 {
		t.Fatalf("payload not parsed: %+v", snapshot)
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), fakeToken) {
		t.Fatal("token leaked into the snapshot JSON")
	}

	if err := Record(dir, "lucy", snapshot); err != nil {
		t.Fatal(err)
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(body), fakeToken) {
			t.Fatalf("token leaked into %s", path)
		}
		return nil
	})
}

func TestFetchOAuthRejectedTokenDoesNotServeCache(t *testing.T) {
	dir := t.TempDir()
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"` + fakeToken + `"}}`))
			return
		}
		w.Write([]byte(`{"five_hour":{"utilization":0.5}}`))
	}))
	defer srv.Close()

	original := oauthEndpoint
	oauthEndpoint = srv.URL
	defer func() { oauthEndpoint = original }()

	if _, err := FetchOAuth(context.Background(), dir, fakeToken); err != nil {
		t.Fatal(err)
	}
	status = http.StatusUnauthorized
	os.Remove(oauthCachePath(dir))
	_, err := FetchOAuth(context.Background(), dir, fakeToken)
	if err == nil || !strings.Contains(err.Error(), "jardin usage login") {
		t.Fatalf("a 401 must name the fix, got %v", err)
	}
	if strings.Contains(err.Error(), fakeToken) {
		t.Fatal("error message echoed the token")
	}
}

func TestFetchOAuthServesCacheOnRateLimit(t *testing.T) {
	dir := t.TempDir()
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Write([]byte(`{"five_hour":{"utilization":0.5}}`))
	}))
	defer srv.Close()

	original := oauthEndpoint
	oauthEndpoint = srv.URL
	defer func() { oauthEndpoint = original }()

	if _, err := FetchOAuth(context.Background(), dir, fakeToken); err != nil {
		t.Fatal(err)
	}
	cache, ok := readOAuthCache(dir)
	if !ok {
		t.Fatal("nothing cached")
	}
	cache.FetchedAt = cache.FetchedAt.Add(-2 * OAuthCacheTTL)
	stale, _ := json.Marshal(cache)
	os.WriteFile(oauthCachePath(dir), stale, 0o644)

	status = http.StatusTooManyRequests
	got, err := FetchOAuth(context.Background(), dir, fakeToken)
	if err != nil {
		t.Fatalf("a 429 must fall back to cache, got %v", err)
	}
	if got.Windows[0].UsedPercentage != 50 {
		t.Fatalf("cached value not served: %+v", got)
	}
}
