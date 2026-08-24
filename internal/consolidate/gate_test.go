package consolidate

import (
	"reflect"
	"testing"
)

func TestGate(t *testing.T) {
	goodText := "the fix was to set OIDC_ISSUER before SSO_ONLY in internal/env, otherwise the process exits 1 at startup"
	tests := []struct {
		name      string
		candidate Candidate
		wantRules []string
	}{
		{
			name:      "pass: actionable specific grounded candidate",
			candidate: Candidate{Text: goodText, EpisodeRefs: []string{"pi/2026-08-24.jsonl#12"}},
			wantRules: nil,
		},
		{
			name:      "fail behavior: short fragment",
			candidate: Candidate{Text: "ok", EpisodeRefs: []string{"pi/x.jsonl"}},
			wantRules: []string{RuleBehavior, RuleObvious},
		},
		{
			name:      "fail behavior: long chatter without action words",
			candidate: Candidate{Text: "we discussed the weather today and it was quite pleasant outside overall, a nice change of pace for the team", EpisodeRefs: []string{"pi/x.jsonl"}},
			wantRules: []string{RuleBehavior, RuleObvious},
		},
		{
			name:      "fail obvious: actionable but generic documentation fact",
			candidate: Candidate{Text: "Go is a programming language that must be used carefully and you should always read its documentation before coding with it", EpisodeRefs: []string{"pi/x.jsonl"}},
			wantRules: []string{RuleObvious},
		},
		{
			name:      "fail grounded: no episode refs",
			candidate: Candidate{Text: goodText},
			wantRules: []string{RuleGrounded},
		},
		{
			name:      "fail secret: credential assignment",
			candidate: Candidate{Text: "remember that API_TOKEN=supersecretvalue works for staging", EpisodeRefs: []string{"pi/x.jsonl"}},
			wantRules: []string{RuleBehavior, RuleObvious, RuleSecret},
		},
		{
			name:      "fail secret: token-shaped literal",
			candidate: Candidate{Text: "the deploy used ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456 as credentials", EpisodeRefs: []string{"pi/x.jsonl"}},
			wantRules: []string{RuleObvious, RuleSecret},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Gate(tt.candidate)
			var rules []string
			for _, r := range got {
				rules = append(rules, r.Rule)
			}
			if !reflect.DeepEqual(rules, tt.wantRules) {
				t.Fatalf("Gate() rules = %v, want %v (rejections: %v)", rules, tt.wantRules, got)
			}
			for _, r := range got {
				if r.Reason == "" {
					t.Errorf("rule %q rejected with empty reason", r.Rule)
				}
			}
		})
	}
}

func TestGateSecretsMatchFlowVectors(t *testing.T) {
	vectors := map[string]string{
		"env assignment":    "API_TOKEN=supersecretvalue",
		"password colon":    "PASSWORD: hunter2000secure",
		"sk- key":           "sk-proj-abcdefghijklmnopqrstuvwx",
		"aws access key":    "AKIAIOSFODNN7EXAMPLE",
		"slack token":       "xoxb-1234567890abcdef",
		"bearer header":     "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"private key block": "-----BEGIN RSA PRIVATE KEY-----",
	}
	for name, vector := range vectors {
		t.Run(name, func(t *testing.T) {
			got := Gate(Candidate{Text: "leak: " + vector, EpisodeRefs: []string{"pi/x.jsonl"}})
			if len(got) == 0 || !containsRule(got, RuleSecret) {
				t.Fatalf("secret vector %q was not rejected under rule %q: %v", vector, RuleSecret, got)
			}
		})
	}
}

func TestGateCleanValuesNotRejected(t *testing.T) {
	text := "set GITHUB_TOKEN from casier before pushing; the flag --dry-run avoids the deploy and the error string `connection refused` means Traefik held the old route on 2026-08-10"
	got := Gate(Candidate{Text: text, EpisodeRefs: []string{"pi/x.jsonl"}})
	if len(got) != 0 {
		t.Fatalf("clean candidate rejected: %v", got)
	}
}

func containsRule(rejections []Rejection, rule string) bool {
	for _, r := range rejections {
		if r.Rule == rule {
			return true
		}
	}
	return false
}
