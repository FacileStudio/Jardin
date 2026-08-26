// Package reports keeps agent-generated HTML pages in the synced tree, so a
// page written on one machine can be opened on another without hosting it.
//
// A report is deliberately not a wiki page. internal/memory indexes, embeds and
// language-checks everything under memory/ and nothing outside it, so reports/
// crosses machines without ever entering the corpus a search answers from.
// A report is derived output carrying an expiry; the wiki is the source of
// truth and keeps its pages forever.
//
// The metadata lives inside the file it describes, as meta tags, rather than in
// a sidecar. internal/sync reconciles one file at a time and writes a .conflict
// beside anything two machines changed, so a shared manifest naming every
// report would collide whenever two machines recorded one in the same minute. A
// self-contained file cannot.
package reports

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Dirname is the reports directory's name under the mycelium data directory.
const Dirname = "reports"

// DefaultTTL is how long a report lives when the caller names no expiry. A
// report is a rendered answer to a question somebody already asked, and the
// synced tree is a wiki that a handful of unswept pages would outweigh.
const DefaultTTL = 30 * 24 * time.Hour

// timeFormat stamps and parses the meta tags.
const timeFormat = time.RFC3339

// Report is one recorded page: what it is called, where it lives and when it
// goes away. A zero Expires means the report was pinned and is never swept.
type Report struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Machine string    `json:"machine"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
}

// Expired reports whether the expiry has passed. A pinned report never has.
func (r Report) Expired(now time.Time) bool {
	return !r.Expires.IsZero() && now.After(r.Expires)
}

// Request describes a page to record. Title falls back to the document's own
// title, and a zero Expires means DefaultTTL rather than never: forgetting the
// flag should cost a report, not leak one.
type Request struct {
	Source  string
	Title   string
	Machine string
	Expires time.Time
	Pinned  bool
}

// Add copies the source page into the reports directory, stamps its metadata
// and returns what it recorded.
//
// The identifier is a slug of the title, so recording the same page twice
// replaces it rather than accumulating a numbered pile. That is the one
// property of a hosted artifact worth keeping without a host: the name you hand
// somebody stays the name of the current version.
func Add(dataDir string, req Request, now time.Time) (Report, error) {
	raw, err := os.ReadFile(req.Source)
	if err != nil {
		return Report{}, fmt.Errorf("failed to read %s: %w", req.Source, err)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFrom(raw, req.Source)
	}
	id := Slug(title)
	if id == "" {
		return Report{}, fmt.Errorf("cannot derive a name from %q, pass --title", req.Source)
	}
	rep := Report{
		ID:      id,
		Title:   title,
		Path:    filepath.Join(Dir(dataDir), id+".html"),
		Machine: req.Machine,
		Created: now,
		Expires: expiryFor(req, now),
	}
	if err := os.MkdirAll(Dir(dataDir), 0o755); err != nil {
		return Report{}, fmt.Errorf("failed to create the reports directory: %w", err)
	}
	if err := os.WriteFile(rep.Path, stamp(raw, rep), 0o644); err != nil {
		return Report{}, fmt.Errorf("failed to write %s: %w", rep.Path, err)
	}
	return rep, nil
}

// expiryFor resolves the requested lifetime. Pinned wins over an explicit date,
// because passing both is a caller changing its mind mid-command and keeping
// the page is the recoverable reading.
func expiryFor(req Request, now time.Time) time.Time {
	if req.Pinned {
		return time.Time{}
	}
	if !req.Expires.IsZero() {
		return req.Expires
	}
	return now.Add(DefaultTTL)
}

var titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// titleFrom prefers the document's own title and falls back to the file name,
// so an agent that named its document well has already named the report.
func titleFrom(raw []byte, source string) string {
	if m := titleTag.FindSubmatch(raw); m != nil {
		if t := strings.TrimSpace(string(m[1])); t != "" {
			return t
		}
	}
	return strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
}

var slugTrim = regexp.MustCompile(`[^a-z0-9]+`)

// Slug reduces a title to the identifier a report is filed and fetched under.
func Slug(title string) string {
	return strings.Trim(slugTrim.ReplaceAllString(strings.ToLower(title), "-"), "-")
}
