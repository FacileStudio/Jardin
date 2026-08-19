package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// The wiki is English-only, by decision rather than by default: retrieval is
// measured on one language and a French page is unreachable from the English
// query an agent actually writes. See ~/.jardin/rules/20-memory.md.
//
// This is the transport-side half of that rule. It warns, and never blocks:
// sync pulls as well as pushes, so refusing on French would lock a machine out
// of the very fix it needs to fetch. The blocking gate belongs where a human is
// present to act on it.

// frenchFunctionWords carries words common in French prose. Several are also
// English ("plus", "sur", "fait"), which is safe because density decides, never
// a single word: a line must clear both an absolute and a proportional
// threshold before it counts.
//
// A composite literal rather than an init(): a package-level var written in its
// own file reads as shared mutable state to filet, and an init() is magic that
// runs before main. Written once here, it is a lookup table and nothing flags it.
var frenchFunctionWords = map[string]bool{
	"afin": true, "ainsi": true, "alors": true, "aux": true, "avait": true,
	"avec": true, "cela": true, "celui": true, "ces": true, "cet": true,
	"cette": true, "ceux": true, "chaque": true, "dans": true, "des": true,
	"deux": true, "doit": true, "donc": true, "dont": true, "déjà": true,
	"elle": true, "encore": true, "entre": true, "est": true, "fait": true,
	"faut": true, "ils": true, "jamais": true, "la": true, "le": true,
	"les": true, "leur": true, "lorsque": true, "mais": true, "même": true,
	"ne": true, "nous": true, "par": true, "parce": true, "pas": true,
	"peut": true, "plus": true, "pour": true, "puis": true, "quand": true,
	"qui": true, "quoi": true, "sans": true, "ses": true, "sont": true,
	"sur": true, "tout": true, "toute": true, "une": true, "vous": true,
	"était": true, "être": true,
}

const (
	langMinWords  = 6
	langMinFrench = 3
	langMinRatio  = 0.12

	// langExemptMarker frees one line, for a verbatim quotation or a French
	// legal string. Narrow on purpose: it is not for prose awaiting translation.
	langExemptMarker = "lang:fr"

	// langExemptFile is append-only by invariant, so its history is never
	// rewritten and its old French entries stay as written.
	langExemptFile = "log.md"
)

// LanguageFinding locates one line of French prose.
type LanguageFinding struct {
	Path string
	Line int
}

// FrenchLines reports the 1-indexed lines of content that read as French prose.
//
// A word-set lookup rather than a regexp, deliberately: the equivalent Go
// regexp — case-insensitive alternation over ~40 words, applied per line — runs
// about 27x slower over the wiki (644ms against 24ms for 237 pages), which is
// too much to spend on a path that syncs every 60 seconds.
func FrenchLines(content string) []int {
	var lines []int
	for i, line := range strings.Split(content, "\n") {
		if lineIsFrench(line) {
			lines = append(lines, i+1)
		}
	}
	return lines
}

// lineIsFrench reports whether one line reads as French prose. Split out of
// FrenchLines so neither function carries the tokenising and the thresholds at
// once, and so the decision can be read on its own.
func lineIsFrench(line string) bool {
	if strings.Contains(line, langExemptMarker) {
		return false
	}
	words, hits := countFrench(line)
	return words >= langMinWords && hits >= langMinFrench &&
		float64(hits)/float64(words) >= langMinRatio
}

// countFrench returns the prose words on a line and how many of them are French.
func countFrench(line string) (words, hits int) {
	for _, token := range strings.Fields(line) {
		if isOpaqueToken(token) {
			continue
		}
		word := strings.ToLower(strings.Trim(token, punctuation))
		if word == "" {
			continue
		}
		words++
		if frenchFunctionWords[word] {
			hits++
		}
	}
	return words, hits
}

// punctuation is trimmed from a token's edges before lookup, so "prose," and
// "(prose)" match. Inner characters are left alone: a token is either prose or
// opaque, never half-tokenised into fragments.
const punctuation = `.,;:!?()[]{}<>"'` + "`" + `*_~«»…—–-`

// isOpaqueToken reports whether a whitespace-separated token is something other
// than a prose word — a URL, a path, an identifier, an inline code span.
//
// This is the whole reason tokenising splits on whitespace rather than on
// non-letters. A CNIL citation such as
// https://www.cnil.fr/fr/la-prospection-commerciale-par-courrier-electronique
// contains "la", "par" and "courrier" as slug fragments; splitting on every
// non-letter turns one URL into a line of French and reports a page whose prose
// is entirely English. Wiki pages cite French sources routinely, so this is the
// false positive that would get the rule switched off.
//
// A token still carrying an inner separator once edge punctuation is stripped is
// an identifier or a slug rather than a word: snake_case, kebab-slug, file.ext.
func isOpaqueToken(token string) bool {
	for _, r := range token {
		switch r {
		case '/', '\\', ':', '@', '=', '#', '$', '%', '+', '|':
			return true
		}
	}
	inner := strings.Trim(token, punctuation)
	return strings.ContainsAny(inner, "_.-")
}

// scanFile reports French prose in one file. reportPath is what a finding
// carries, which differs by caller: sync reports data-dir-relative paths so they
// match its own output, doctor reports wiki-relative ones. The exemption is
// decided on the base name so neither caller can lose it by passing a different
// shape of path.
func scanFile(absPath, reportPath string) []LanguageFinding {
	if filepath.Base(absPath) == langExemptFile {
		return nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		// A warning path must never be the reason a sync or a doctor run fails.
		return nil
	}
	var findings []LanguageFinding
	for _, line := range FrenchLines(string(data)) {
		findings = append(findings, LanguageFinding{Path: reportPath, Line: line})
	}
	return findings
}

// ScanPaths reports French prose in the given data-dir-relative paths. Callers
// pass the files a sync actually touched rather than the whole corpus, so the
// usual cost is a handful of files or none. Anything that is not a wiki markdown
// page is skipped.
func ScanPaths(dataDir string, rels []string) []LanguageFinding {
	var findings []LanguageFinding
	for _, rel := range rels {
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "memory/") || !strings.HasSuffix(rel, ".md") {
			continue
		}
		findings = append(findings,
			scanFile(filepath.Join(dataDir, filepath.FromSlash(rel)), rel)...)
	}
	return findings
}

// ScanWiki reports French prose across every page under memoryDir. It is the
// whole-corpus counterpart to ScanPaths, and exists so that one implementation
// answers both questions — a second copy of this rule in another language would
// drift, and did: an earlier TypeScript checker tokenised on whitespace, which
// counted identifiers and URLs as prose words and hid four French index hooks
// under the ratio threshold.
//
// It walks and reads directly rather than delegating to ScanPaths, because that
// would make the result depend on memoryDir being named "memory" — and a check
// that silently reports clean when it cannot see the corpus is worse than none.
func ScanWiki(memoryDir string) ([]LanguageFinding, error) {
	var findings []LanguageFinding
	err := filepath.Walk(memoryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(memoryDir, path)
		if relErr != nil {
			return nil
		}
		findings = append(findings, scanFile(path, filepath.ToSlash(rel))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}
