package consolidate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJudgeVerdicts(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		handler      http.HandlerFunc
		wantVerdict  Verdict
		wantJudgedBy string
	}{
		{
			name:         "accept on yes",
			model:        "llama3.2:3b",
			handler:      ollamaReply("YES"),
			wantVerdict:  VerdictAccept,
			wantJudgedBy: "ollama:llama3.2:3b",
		},
		{
			name:         "accept on lowercase yes with padding",
			model:        "llama3.2:3b",
			handler:      ollamaReply("\n  yes.\n"),
			wantVerdict:  VerdictAccept,
			wantJudgedBy: "ollama:llama3.2:3b",
		},
		{
			name:         "refuse on no",
			model:        "llama3.2:3b",
			handler:      ollamaReply("NO"),
			wantVerdict:  VerdictRefuse,
			wantJudgedBy: "ollama:llama3.2:3b",
		},
		{
			name:         "refuse on unrecognized answer",
			model:        "llama3.2:3b",
			handler:      ollamaReply("maybe?"),
			wantVerdict:  VerdictRefuse,
			wantJudgedBy: "ollama:llama3.2:3b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			j := NewJudge(srv.URL, tt.model)
			got := j.Judge(context.Background(), Candidate{Text: "the fix was to backfill oidc_subject"})
			if got.Verdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.wantVerdict)
			}
			if got.JudgedBy != tt.wantJudgedBy {
				t.Errorf("judgedBy = %q, want %q", got.JudgedBy, tt.wantJudgedBy)
			}
		})
	}
}

func TestJudgeFailsOpen(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		model   string
	}{
		{name: "unreachable server", baseURL: "http://127.0.0.1:1", model: "llama3.2:3b"},
		{name: "unconfigured model", baseURL: "http://127.0.0.1:1", model: ""},
		{name: "unconfigured url", baseURL: "", model: "llama3.2:3b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := NewJudge(tt.baseURL, tt.model)
			got := j.Judge(context.Background(), Candidate{Text: "gotcha: chi decodes path params"})
			if got.Verdict != VerdictAccept {
				t.Errorf("verdict = %q, want accept (fail open)", got.Verdict)
			}
			if got.JudgedBy != JudgedByFallback {
				t.Errorf("judgedBy = %q, want %q", got.JudgedBy, JudgedByFallback)
			}
		})
	}
}

func TestJudgeFailsOpenOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j := NewJudge(srv.URL, "llama3.2:3b")
	got := j.Judge(context.Background(), Candidate{Text: "note that the ledger dedupes by requestId"})
	if got.Verdict != VerdictAccept || got.JudgedBy != JudgedByFallback {
		t.Errorf("server error should fail open, got %+v", got)
	}
}

func TestJudgePromptAndRequestShape(t *testing.T) {
	var received struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"response": "YES"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	j := NewJudge(srv.URL, "llama3.2:3b")
	j.Judge(context.Background(), Candidate{Text: "turns out the watermark lives in DataDir"})
	if received.Model != "llama3.2:3b" {
		t.Errorf("model = %q", received.Model)
	}
	if received.Stream {
		t.Error("stream should be false")
	}
	if !strings.Contains(received.Prompt, "useful in 30 days") ||
		!strings.Contains(received.Prompt, "turns out the watermark lives in DataDir") {
		t.Errorf("prompt missing question or finding: %q", received.Prompt)
	}
}

func ollamaReply(response string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]string{"response": response}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
