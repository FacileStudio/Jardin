package consolidate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestApplyKeepsASecretAwayFromTheJudge is the ordering guarantee. The judge is
// an HTTP call and OLLAMA_URL can name another host, so a candidate the secret
// rule is about to reject must never leave the process. Every other rejection
// still pays for a verdict.
func TestApplyKeepsASecretAwayFromTheJudge(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"response":"YES"}`))
	}))
	defer srv.Close()

	p := &pipeline{
		ctx:     context.Background(),
		judge:   NewJudge(srv.URL, "llama3.2:3b"),
		dataDir: t.TempDir(),
		now:     writeNow,
	}
	res := Result{}
	p.apply(Candidate{
		Text:        "the deploy failed until GITHUB_TOKEN=ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa was set in docker-compose.yml",
		EpisodeRefs: []string{"pi/2026-08.jsonl:1"},
	}, &res)

	if calls.Load() != 0 {
		t.Fatalf("the judge saw a candidate carrying a credential (%d calls)", calls.Load())
	}
	if res.Dropped != 1 || !strings.Contains(strings.Join(res.Reasons, "\n"), RuleSecret) {
		t.Fatalf("the secret rule must still reject and report: %+v", res)
	}

	res = Result{}
	p.apply(Candidate{Text: "just a note here", EpisodeRefs: []string{"pi/2026-08.jsonl:2"}}, &res)
	if calls.Load() == 0 {
		t.Fatal("a candidate with no secret must still reach the judge")
	}
}
