// Package consolidate turns episodic records into semantic memory pages.
// gate.go is the executable storage gate: every candidate must clear all four
// rules from the storage-gate policy before it may be written into memory/.
package consolidate

import (
	"fmt"
	"regexp"
	"strings"
)

// Rule names returned in a Rejection, one per storage-gate clause.
const (
	RuleBehavior = "behavior"
	RuleObvious  = "obvious"
	RuleGrounded = "grounded"
	RuleSecret   = "secret"
)

const (
	// MinBehaviorLength keeps one-word fragments out of rule 1 regardless of
	// what keywords they happen to contain.
	MinBehaviorLength = 30
	// MinSpecificitySignals is how many concrete anchors (paths, dates, error
	// strings, versions) a candidate needs to count as non-obvious.
	MinSpecificitySignals = 1
	// secretMarkers mirrors internal/flow's isSecretName name heuristics: an
	// assignment whose key carries one of these substrings is treated as a
	// credential whatever its value looks like.
	secretMarkers = "TOKEN|SECRET|KEY|PASSWORD|CREDENTIAL|PASSWD|APIKEY"
)

var (
	behaviorWords = map[string]bool{
		"fix": true, "fixed": true, "instead": true, "avoid": true, "must": true,
		"use": true, "never": true, "always": true, "configure": true, "set": true,
		"enable": true, "disable": true, "breaks": true, "fails": true,
		"requires": true, "rejects": true, "refuses": true, "default": true,
		"override": true, "migrate": true, "rename": true, "broke": true,
	}
	pathRe    = regexp.MustCompile(`(/[\w.-]+){2,}|[\w.-]+\.(go|ts|md|json|ya?ml|toml)`)
	dateRe    = regexp.MustCompile(`\b(20\d{2}-\d{2}-\d{2})\b`)
	errorRe   = regexp.MustCompile("`[^`]{8,}`|[A-Za-z]+Error\\b|exit status \\d+|\\w+\\.\\w+: .{10,}")
	versionRe = regexp.MustCompile(`\bv?\d+\.\d+(\.\d+)?\b`)
	assignRe  = regexp.MustCompile(`(?i)\b[A-Z0-9_]*(` + secretMarkers + `)[A-Z0-9_]*\s*[:=]\s*\S{4,}`)
	tokenRe   = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{16,}|ghp_[A-Za-z0-9]{20,}|gho_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[bpars]-[A-Za-z0-9-]{10,}|Bearer [A-Za-z0-9._-]{20,})\b`)
	pemRe     = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
)

// Rejection explains why the gate refused a candidate. Rule names the failed
// storage-gate clause, Reason is human-readable and safe to log.
type Rejection struct {
	Rule   string
	Reason string
}

func (r Rejection) Error() string {
	return fmt.Sprintf("%s: %s", r.Rule, r.Reason)
}

// Gate applies the four storage-gate rules to a candidate and returns one
// rejection per failed rule, in rule order. An empty slice means the
// candidate may enter memory/.
//
// The rules mirror ~/.mycelium/rules/20-memory.md:
//
//  1. behavior — the text plausibly changes how a future agent acts
//  2. obvious  — not trivially rediscoverable from code or command output
//  3. grounded — carries episode references as provenance
//  4. secret   — no credentials, tokens, keys or passwords, ever
func Gate(c Candidate) []Rejection {
	var out []Rejection
	if r := checkBehavior(c.Text); r != nil {
		out = append(out, *r)
	}
	if r := checkObvious(c.Text); r != nil {
		out = append(out, *r)
	}
	if len(c.EpisodeRefs) == 0 {
		out = append(out, Rejection{
			Rule:   RuleGrounded,
			Reason: "no episode references: a claim without provenance cannot be verified or superseded later",
		})
	}
	if r := checkSecrets(c.Text); r != nil {
		out = append(out, *r)
	}
	return out
}

// checkBehavior rejects chatter: text too short to act on, or with no word
// that implies a decision, constraint or correction a future agent would need.
func checkBehavior(text string) *Rejection {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < MinBehaviorLength {
		return &Rejection{
			Rule:   RuleBehavior,
			Reason: fmt.Sprintf("too short (%d chars) to change future behavior", len(trimmed)),
		}
	}
	lowered := strings.ToLower(trimmed)
	for w := range behaviorWords {
		if strings.Contains(lowered, w) {
			return nil
		}
	}
	for _, m := range gotchaMarkers {
		if strings.Contains(lowered, m) {
			return nil
		}
	}
	return &Rejection{
		Rule:   RuleBehavior,
		Reason: "no actionable language: reads as commentary rather than something a future agent would act on",
	}
}

// checkObvious rejects facts a future agent could rerun or read straight out
// of the current code. Specificity signals (file paths, dates, error strings,
// version numbers, gotcha markers) stand in for "annoying to rediscover".
func checkObvious(text string) *Rejection {
	signals := 0
	for _, re := range []*regexp.Regexp{pathRe, dateRe, errorRe, versionRe} {
		if re.MatchString(text) {
			signals++
		}
	}
	lowered := strings.ToLower(text)
	for _, m := range gotchaMarkers {
		if strings.Contains(lowered, m) {
			signals++
			break
		}
	}
	if signals >= MinSpecificitySignals {
		return nil
	}
	return &Rejection{
		Rule:   RuleObvious,
		Reason: "no specificity signal (path, date, error string, version or gotcha): trivially rediscoverable",
	}
}

// checkSecrets refuses credentials outright, whatever the other three rules
// said. It combines two detectors: flow's env-name heuristic applied to
// assignments inside the text, and structural token shapes that leak even
// under an innocent variable name.
func checkSecrets(text string) *Rejection {
	if loc := assignRe.FindString(text); loc != "" {
		return &Rejection{
			Rule:   RuleSecret,
			Reason: fmt.Sprintf("looks like a credential assignment (%q); credentials are refused whatever the other rules say", truncate(loc)),
		}
	}
	if loc := tokenRe.FindString(text); loc != "" {
		return &Rejection{
			Rule:   RuleSecret,
			Reason: fmt.Sprintf("contains a token-shaped literal (%q…)", truncate(loc)),
		}
	}
	if pemRe.MatchString(text) {
		return &Rejection{
			Rule:   RuleSecret,
			Reason: "contains a private key block",
		}
	}
	return nil
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 24 {
		return s[:21] + "..."
	}
	return s
}
