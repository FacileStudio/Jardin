package memory

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
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
var frenchFunctionWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(
		`le la les des une est qui pour dans sont avec cette donc mais pas ne
		 leur ces aux par sur ses elle nous vous ils ainsi quand alors entre
		 sans plus tout fait deux chaque dont puis encore faut peut doit
		 était avait cet ceux quoi parce lorsque afin`) {
		frenchFunctionWords[w] = true
	}
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
		if strings.Contains(line, langExemptMarker) {
			continue
		}
		words, hits := 0, 0
		for _, w := range strings.FieldsFunc(line, func(r rune) bool { return !unicode.IsLetter(r) }) {
			words++
			if frenchFunctionWords[strings.ToLower(w)] {
				hits++
			}
		}
		if words >= langMinWords && hits >= langMinFrench &&
			float64(hits)/float64(words) >= langMinRatio {
			lines = append(lines, i+1)
		}
	}
	return lines
}

// ScanPaths reports French prose in the given wiki-relative paths, which are
// resolved under dataDir. Callers pass the files a sync actually touched rather
// than the whole corpus, so the usual cost is a handful of files or none.
// Anything that is not a wiki markdown page is skipped, as is an unreadable
// file: this is a warning path and must never be the reason a sync fails.
func ScanPaths(dataDir string, rels []string) []LanguageFinding {
	var findings []LanguageFinding
	for _, rel := range rels {
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "memory/") || !strings.HasSuffix(rel, ".md") {
			continue
		}
		if strings.HasSuffix(rel, "/"+langExemptFile) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, line := range FrenchLines(string(data)) {
			findings = append(findings, LanguageFinding{Path: rel, Line: line})
		}
	}
	return findings
}
