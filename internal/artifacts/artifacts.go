package artifacts

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const Dirname = "artifacts"

const DefaultTTL = 30 * 24 * time.Hour

const timeFormat = time.RFC3339

// Artifact represents a recorded markdown or HTML document.
type Artifact struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Machine string    `json:"machine"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
}

// Report is a backward-compatible alias for Artifact.
type Report = Artifact

// Expired reports whether the artifact has passed its expiry.
func (a Artifact) Expired(now time.Time) bool {
	return !a.Expires.IsZero() && now.After(a.Expires)
}

// Request holds parameters for adding a new artifact.
type Request struct {
	Source  string
	Title   string
	Machine string
	Expires time.Time
	Pinned  bool
}

// Add copies and registers a new artifact in the local store.
func Add(dataDir string, req Request, now time.Time) (Artifact, error) {
	raw, err := os.ReadFile(req.Source)
	if err != nil {
		return Artifact{}, fmt.Errorf("failed to read %s: %w", req.Source, err)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFrom(raw, req.Source)
	}
	id := idFor(title, req.Machine, now)
	if id == "" {
		return Artifact{}, fmt.Errorf("cannot derive a name from %q, pass --title", req.Source)
	}
	ext := ".md"
	if isHTML(req.Source, raw) {
		ext = ".html"
	}
	targetDir := filepath.Join(Dir(dataDir), now.UTC().Format("2006/01"))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return Artifact{}, fmt.Errorf("failed to create target directory: %w", err)
	}
	art := Artifact{
		ID:      id,
		Title:   title,
		Path:    filepath.Join(targetDir, id+ext),
		Machine: req.Machine,
		Created: now,
		Expires: expiryFor(req, now),
	}
	stamped := stamp(raw, art, ext == ".html")
	if err := os.WriteFile(art.Path, stamped, 0o644); err != nil {
		return Artifact{}, fmt.Errorf("failed to write %s: %w", art.Path, err)
	}
	return art, nil
}

func expiryFor(req Request, now time.Time) time.Time {
	if req.Pinned {
		return time.Time{}
	}
	if !req.Expires.IsZero() {
		return req.Expires
	}
	return now.Add(DefaultTTL)
}

var (
	titleTag     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	mdHeading    = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	slugTrim     = regexp.MustCompile(`[^a-z0-9]+`)
	datePrefix   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)
	fullHashedID = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-.+-[a-f0-9]{6}$`)
)

func idFor(title, machine string, now time.Time) string {
	slug := Slug(title)
	if slug == "" {
		return ""
	}
	if fullHashedID.MatchString(slug) {
		return slug
	}
	baseSlug := datePrefix.ReplaceAllString(slug, "")
	h := sha256.Sum256([]byte(machine + ":" + title + ":" + now.UTC().Format(time.RFC3339Nano)))
	hashStr := fmt.Sprintf("%x", h[:3])
	return now.UTC().Format("2006-01-02") + "-" + baseSlug + "-" + hashStr
}

func isHTML(source string, raw []byte) bool {
	if strings.HasSuffix(strings.ToLower(source), ".html") || strings.HasSuffix(strings.ToLower(source), ".htm") {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	return bytes.HasPrefix(trimmed, []byte("<!DOCTYPE")) || bytes.HasPrefix(trimmed, []byte("<html"))
}

func titleFrom(raw []byte, source string) string {
	if isHTML(source, raw) {
		if m := titleTag.FindSubmatch(raw); m != nil {
			if t := strings.TrimSpace(string(m[1])); t != "" {
				return t
			}
		}
		return strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	}
	text := string(raw)
	if title := extractFrontmatterTitle(text); title != "" {
		return title
	}
	if m := mdHeading.FindStringSubmatch(text); len(m) > 1 {
		if t := strings.TrimSpace(m[1]); t != "" {
			return t
		}
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return datePrefix.ReplaceAllString(base, "")
}

func extractFrontmatterTitle(content string) string {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return ""
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return ""
	}
	head := content[4 : 4+end]
	for _, line := range strings.Split(head, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "title:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "title:"))
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

// Slug sanitizes a title into a URL and filename friendly slug.
func Slug(title string) string {
	return strings.Trim(slugTrim.ReplaceAllString(strings.ToLower(title), "-"), "-")
}
