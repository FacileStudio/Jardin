package consolidate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	judgeTimeout     = 30 * time.Second
	judgeBodyExcerpt = 256
	JudgedByFallback = "heuristic-fallback"
	judgeModelPrefix = "ollama:"
)

// Verdict is the judge's binary outcome.
type Verdict string

const (
	VerdictAccept Verdict = "accept"
	VerdictRefuse Verdict = "refuse"
)

// Judgement pairs the verdict with what produced it: the local model, or the
// fail-open fallback when Ollama was unreachable or unconfigured.
type Judgement struct {
	Verdict  Verdict
	JudgedBy string
}

// Judge asks a small local Ollama model one yes/no question about a candidate:
// will this finding still be useful in 30 days? A zero-value or empty model is
// fallback mode: every candidate is accepted without a round trip.
type Judge struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewJudge builds a judge talking to Ollama at baseURL using the named model.
// An empty model disables judging entirely; the client carries a timeout
// because a model that hangs while loading would otherwise wedge a daemon tick.
func NewJudge(baseURL, model string) *Judge {
	return &Judge{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   strings.TrimSpace(model),
		client:  &http.Client{Timeout: judgeTimeout},
	}
}

// Judge returns the verdict for one candidate. Infrastructure failures never
// surface as errors: an unreachable or misbehaving Ollama degrades to
// heuristic-fallback acceptance, because the storage gate still applies
// downstream and consolidation must work offline.
func (j *Judge) Judge(ctx context.Context, cand Candidate) Judgement {
	if j.model == "" || j.baseURL == "" {
		return Judgement{Verdict: VerdictAccept, JudgedBy: JudgedByFallback}
	}
	verdict := j.verdict(ctx, judgePrompt(cand.Text))
	if verdict.JudgedBy == JudgedByFallback {
		return Judgement{Verdict: VerdictAccept, JudgedBy: JudgedByFallback}
	}
	return verdict
}

// Compare asks the model whether a candidate contradicts an existing claim.
// VerdictAccept means "yes, contradicted". Unlike Judge, this fails CLOSED:
// an unreachable or unconfigured model returns VerdictRefuse, because a
// contradiction nobody could confirm must degrade to NOOP, never to striking
// a claim in the wiki. Verifying a retraction is exactly the case where
// "could not check" is a reason to refuse.
func (j *Judge) Compare(ctx context.Context, existing, candidate string) Judgement {
	if j.model == "" || j.baseURL == "" {
		return Judgement{Verdict: VerdictRefuse, JudgedBy: JudgedByFallback}
	}
	return j.verdict(ctx, comparePrompt(existing, candidate))
}

func (j *Judge) verdict(ctx context.Context, prompt string) Judgement {
	yes, err := j.ask(ctx, prompt)
	if err != nil {
		return Judgement{Verdict: VerdictRefuse, JudgedBy: JudgedByFallback}
	}
	if !yes {
		return Judgement{Verdict: VerdictRefuse, JudgedBy: j.judgedBy()}
	}
	return Judgement{Verdict: VerdictAccept, JudgedBy: j.judgedBy()}
}

func (j *Judge) judgedBy() string { return judgeModelPrefix + j.model }

func comparePrompt(existing, candidate string) string {
	var b strings.Builder
	b.WriteString("You are a consistency judge for a personal knowledge wiki. ")
	b.WriteString("Answer with exactly one word: YES or NO.\n")
	b.WriteString("Question: does the new finding state something the existing claim denies or says is no longer true?\n\n")
	b.WriteString("Existing claim:\n")
	b.WriteString(existing)
	b.WriteString("\n\nNew finding:\n")
	b.WriteString(candidate)
	return b.String()
}

func (j *Judge) ask(ctx context.Context, prompt string) (bool, error) {
	body := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}{
		Model:  j.model,
		Prompt: prompt,
		Stream: false,
	}

	var reply struct {
		Response string `json:"response"`
	}
	if err := j.post(ctx, "/api/generate", body, &reply); err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(reply.Response))
	return strings.HasPrefix(answer, "yes"), nil
}

func judgePrompt(text string) string {
	var b strings.Builder
	b.WriteString("You are a durability judge for a personal knowledge wiki. ")
	b.WriteString("Answer with exactly one word: YES or NO.\n")
	b.WriteString("Question: will this finding still be useful in 30 days?\n\n")
	b.WriteString("Finding:\n")
	b.WriteString(text)
	return b.String()
}

func (j *Judge) post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("consolidate judge: %s: encode request: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("consolidate judge: %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("consolidate judge: %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, judgeBodyExcerpt))
		return fmt.Errorf("consolidate judge: %s: status %d: %s", path, resp.StatusCode,
			strings.TrimSpace(string(excerpt)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("consolidate judge: %s: decode response: %w", path, err)
	}
	return nil
}
