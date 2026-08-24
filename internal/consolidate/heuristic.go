package consolidate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CandidateKind classifies which heuristic pattern produced a candidate.
type CandidateKind int

const (
	KindErrorFix CandidateKind = iota
	KindGotcha
	KindRepeatedFailure
)

func (k CandidateKind) String() string {
	switch k {
	case KindErrorFix:
		return "error-fix"
	case KindGotcha:
		return "gotcha"
	case KindRepeatedFailure:
		return "repeated-failure"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// Candidate is one proposed durable memory: the finding text, the episode
// references that produced it, and which heuristic surfaced it.
type Candidate struct {
	Text        string
	EpisodeRefs []string
	Kind        CandidateKind
}

const fixWindowLines = 5

var errorIndicators = []string{
	"error:", "fatal:", "panic:", "traceback", "exception",
	"failed to", "failure", "does not compile", "compile error",
	"test failed", "build failed", "permission denied", "no such file",
	"command not found", "connection refused", "timed out",
}

var resolutionIndicators = []string{
	"fixed", "the fix was", "resolved", "solved", "root cause",
	"workaround", "turns out", "solution",
}

var gotchaMarkers = []string{
	"gotcha", "turns out", "the fix was", "note that",
	"watch out", "careful:", "do not forget",
}

// Propose runs every heuristic over the episodes and returns the merged
// candidate list, unjudged and ungated.
func Propose(episodes []Episode) []Candidate {
	var candidates []Candidate
	candidates = append(candidates, errorFixCandidates(episodes)...)
	candidates = append(candidates, gotchaCandidates(episodes)...)
	candidates = append(candidates, repeatedFailureCandidates(episodes)...)
	return candidates
}

func errorFixCandidates(episodes []Episode) []Candidate {
	var out []Candidate
	for _, ep := range episodes {
		out = append(out, episodeErrorFixes(ep)...)
	}
	return out
}

func episodeErrorFixes(ep Episode) []Candidate {
	lines := strings.Split(ep.Text, "\n")
	var out []Candidate
	for i, line := range lines {
		end := i + fixWindowLines + 1
		if end > len(lines) {
			end = len(lines)
		}
		if matchesAny(line, errorIndicators) && hasResolution(lines[i+1:end]) {
			out = append(out, Candidate{
				Text:        strings.TrimSpace(line),
				EpisodeRefs: []string{refOf(ep)},
				Kind:        KindErrorFix,
			})
		}
	}
	return out
}

func hasResolution(lines []string) bool {
	for _, follow := range lines {
		if matchesAny(follow, resolutionIndicators) {
			return true
		}
	}
	return false
}

func gotchaCandidates(episodes []Episode) []Candidate {
	var out []Candidate
	for _, ep := range episodes {
		for _, line := range strings.Split(ep.Text, "\n") {
			if matchesAny(line, gotchaMarkers) {
				out = append(out, Candidate{
					Text:        strings.TrimSpace(line),
					EpisodeRefs: []string{refOf(ep)},
					Kind:        KindGotcha,
				})
			}
		}
	}
	return out
}

type failureGroup struct {
	signature  string
	example    string
	references []string
	timestamps []time.Time
}

func repeatedFailureCandidates(episodes []Episode) []Candidate {
	groups := map[string]*failureGroup{}
	var order []string

	for _, ep := range episodes {
		for _, line := range strings.Split(ep.Text, "\n") {
			if !matchesAny(line, errorIndicators) {
				continue
			}
			sig := signatureOf(line)
			g, ok := groups[sig]
			if !ok {
				g = &failureGroup{signature: sig, example: strings.TrimSpace(line)}
				groups[sig] = g
				order = append(order, sig)
			}
			g.references = append(g.references, refOf(ep))
			g.timestamps = append(g.timestamps, ep.Timestamp)
		}
	}

	sort.Strings(order)
	var out []Candidate
	for _, sig := range order {
		g := groups[sig]
		if distinctTimestamps(g.timestamps) < 2 {
			continue
		}
		out = append(out, Candidate{
			Text:        g.example,
			EpisodeRefs: dedupeStrings(g.references),
			Kind:        KindRepeatedFailure,
		})
	}
	return out
}

func matchesAny(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func signatureOf(line string) string {
	lower := strings.ToLower(line)
	var b strings.Builder
	prevSpace := true
	for _, r := range lower {
		if r >= '0' && r <= '9' {
			r = '#'
		}
		if r == ' ' || r == '\t' {
			if prevSpace {
				continue
			}
			prevSpace = true
			b.WriteRune(' ')
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func distinctTimestamps(ts []time.Time) int {
	seen := map[string]bool{}
	for _, t := range ts {
		seen[t.UTC().Format(time.RFC3339)] = true
	}
	return len(seen)
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func refOf(ep Episode) string {
	return fmt.Sprintf("%s@%s", ep.Agent, ep.Timestamp.UTC().Format(time.RFC3339))
}
