package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func remoteServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestSearchRemoteHappyPath(t *testing.T) {
	var got struct {
		Query   string `json:"query"`
		SpaceID string `json:"space_id"`
		Limit   int    `json:"limit"`
	}
	var auth, path string
	url := remoteServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writeFakeJSON(w, map[string]any{"results": []map[string]any{
			{"path": "tools/jardin.md", "line": 12, "score": 0.0164, "excerpt": "a finding"},
		}})
	})

	results, degraded, err := SearchRemote(context.Background(), RemoteSearch{
		BaseURL: url + "/", Token: "tok", SpaceID: "space-1", Query: "bm25 ranking", Limit: 7,
	})
	if err != nil {
		t.Fatalf("SearchRemote: %v", err)
	}
	if degraded {
		t.Errorf("degraded = true, want false")
	}
	if path != remoteSearchPath || auth != "Bearer tok" {
		t.Errorf("request = %s %q, want %s with bearer token", path, auth, remoteSearchPath)
	}
	if got.Query != "bm25 ranking" || got.SpaceID != "space-1" || got.Limit != 7 {
		t.Errorf("body = %+v, want the query, space and limit passed through", got)
	}
	if len(results) != 1 || results[0].Path != "tools/jardin.md" || results[0].Line != 12 {
		t.Fatalf("results = %+v, want one hit for tools/jardin.md:12", results)
	}
	if results[0].Content != "a finding" || results[0].Score != int(0.0164*remoteScoreScale) {
		t.Errorf("result = %+v, want the excerpt as content and a scaled score", results[0])
	}
}

func TestSearchRemoteDegradedIsSurfaced(t *testing.T) {
	url := remoteServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeFakeJSON(w, map[string]any{"results": []map[string]any{}, "degraded": true})
	})

	results, degraded, err := SearchRemote(context.Background(), RemoteSearch{BaseURL: url, Query: "q"})
	if err != nil {
		t.Fatalf("SearchRemote: %v", err)
	}
	if !degraded {
		t.Errorf("degraded = false, want true when the server reports it")
	}
	if len(results) != 0 {
		t.Errorf("results = %+v, want none", results)
	}
}

func TestSearchRemoteRejectsBadResponses(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"non-200": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"malformed body": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{not json"))
		},
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			url := remoteServer(t, handler)
			if _, _, err := SearchRemote(context.Background(), RemoteSearch{BaseURL: url, Query: "q"}); err == nil {
				t.Fatalf("SearchRemote succeeded, want an error")
			}
		})
	}
}

func TestSearchRemoteWithoutBaseURL(t *testing.T) {
	if _, _, err := SearchRemote(context.Background(), RemoteSearch{Query: "q"}); err == nil {
		t.Fatalf("SearchRemote succeeded without a server, want an error")
	}
}

func TestSearchRemoteHonoursContext(t *testing.T) {
	release := make(chan struct{})
	url := remoteServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
	})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := SearchRemote(ctx, RemoteSearch{BaseURL: url, Query: "q"})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("SearchRemote succeeded, want the cancelled context to abort it")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("SearchRemote did not return after its context expired")
	}
}
