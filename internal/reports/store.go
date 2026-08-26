package reports

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Dir is the reports directory inside a mycelium data directory.
func Dir(dataDir string) string { return filepath.Join(dataDir, Dirname) }

var (
	metaAny  = regexp.MustCompile(`(?i)\n?<meta name="mycelium-[a-z]+" content="[^"]*">`)
	metaPair = regexp.MustCompile(`(?i)<meta name="mycelium-([a-z]+)" content="([^"]*)">`)
	headEnd  = regexp.MustCompile(`(?i)</head>`)
)

// stamp replaces whatever metadata the page already carries with this report's
// own, so re-recording a page pulled back out of reports/ does not accumulate a
// stack of stale tags.
func stamp(raw []byte, rep Report) []byte {
	body := metaAny.ReplaceAll(raw, nil)
	block := metaBlock(rep)
	if loc := headEnd.FindIndex(body); loc != nil {
		out := append([]byte{}, body[:loc[0]]...)
		return append(append(out, block...), body[loc[0]:]...)
	}
	return append(block, body...)
}

// metaBlock renders the tags. Attribute values are HTML-escaped because a title
// is free text and a quote in it would otherwise close the attribute early.
func metaBlock(rep Report) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-title\" content=\"%s\">", html.EscapeString(rep.Title))
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-machine\" content=\"%s\">", html.EscapeString(rep.Machine))
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-created\" content=\"%s\">", rep.Created.UTC().Format(timeFormat))
	if !rep.Expires.IsZero() {
		fmt.Fprintf(&b, "\n<meta name=\"mycelium-expires\" content=\"%s\">", rep.Expires.UTC().Format(timeFormat))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// parse reads back what stamp wrote. An unparseable or missing tag leaves its
// field zero rather than failing the listing: a report whose metadata somebody
// hand-edited should still be visible enough to delete.
func parse(path string, raw []byte) Report {
	rep := Report{ID: strings.TrimSuffix(filepath.Base(path), ".html"), Path: path}
	for _, m := range metaPair.FindAllSubmatch(raw, -1) {
		value := html.UnescapeString(string(m[2]))
		switch strings.ToLower(string(m[1])) {
		case "title":
			rep.Title = value
		case "machine":
			rep.Machine = value
		case "created":
			rep.Created, _ = time.Parse(timeFormat, value)
		case "expires":
			rep.Expires, _ = time.Parse(timeFormat, value)
		}
	}
	if rep.Title == "" {
		rep.Title = rep.ID
	}
	return rep
}

// List returns every report on this machine, newest first.
func List(dataDir string) ([]Report, error) {
	entries, err := os.ReadDir(Dir(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read the reports directory: %w", err)
	}
	var out []Report
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		path := filepath.Join(Dir(dataDir), e.Name())
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		out = append(out, parse(path, raw))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Find resolves one report by its identifier.
func Find(dataDir, id string) (Report, error) {
	path := filepath.Join(Dir(dataDir), Slug(id)+".html")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("no report named %q", id)
	}
	return parse(path, raw), nil
}

// Remove deletes one report by its identifier.
func Remove(dataDir, id string) error {
	rep, err := Find(dataDir, id)
	if err != nil {
		return err
	}
	if err := os.Remove(rep.Path); err != nil {
		return fmt.Errorf("failed to delete %s: %w", rep.Path, err)
	}
	return nil
}

// Sweep deletes every expired report and returns what it took, so the caller
// can say so rather than quietly shrinking the list somebody was reading.
func Sweep(dataDir string, now time.Time) ([]string, error) {
	all, err := List(dataDir)
	if err != nil {
		return nil, err
	}
	var swept []string
	for _, rep := range all {
		if !rep.Expired(now) {
			continue
		}
		if err := os.Remove(rep.Path); err != nil {
			continue
		}
		swept = append(swept, rep.ID)
	}
	return swept, nil
}

var (
	assetRef  = regexp.MustCompile(`(?i)(?:src|href)="([^"]+)"`)
	schemeRef = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)
)

// ExternalRefs returns the relative asset paths a page depends on.
//
// A page opened from disk cannot fetch its siblings, so every one of these is a
// missing stylesheet or a broken image that the reader sees and the author
// never does. Absolute URLs and data: URIs are left alone: they resolve.
func ExternalRefs(raw []byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range assetRef.FindAllSubmatch(raw, -1) {
		ref := strings.TrimSpace(string(m[1]))
		if ref == "" || seen[ref] || strings.HasPrefix(ref, "#") || schemeRef.MatchString(ref) {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}
