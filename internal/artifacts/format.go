package artifacts

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	metaAny  = regexp.MustCompile(`(?i)\n?<meta name="mycelium-[a-z]+" content="[^"]*">`)
	metaPair = regexp.MustCompile(`(?i)<meta name="mycelium-([a-z]+)" content="([^"]*)">`)
	headEnd  = regexp.MustCompile(`(?i)</head>`)
	mdLink   = regexp.MustCompile(`!?\[.*?\]\(([^)]+)\)`)
)

func stamp(raw []byte, art Artifact, isHTML bool) []byte {
	if isHTML {
		body := metaAny.ReplaceAll(raw, nil)
		block := metaBlock(art)
		if loc := headEnd.FindIndex(body); loc != nil {
			out := append([]byte{}, body[:loc[0]]...)
			return append(append(out, block...), body[loc[0]:]...)
		}
		return append(block, body...)
	}

	content := string(raw)
	body := stripFrontmatter(content)
	header := frontmatterBlock(art)
	return []byte(header + body)
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return content
	}
	rest := content[4+end:]
	if cut := strings.Index(rest, "\n"); cut >= 0 {
		rest = rest[cut+1:]
	}
	return strings.TrimPrefix(rest, "\r\n")
}

func frontmatterBlock(art Artifact) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %q\n", art.Title)
	if art.Machine != "" {
		fmt.Fprintf(&b, "machine: %q\n", art.Machine)
	}
	fmt.Fprintf(&b, "created: %s\n", art.Created.UTC().Format(timeFormat))
	if !art.Expires.IsZero() {
		fmt.Fprintf(&b, "expires: %s\n", art.Expires.UTC().Format(timeFormat))
	}
	b.WriteString("type: artifact\n")
	b.WriteString("---\n\n")
	return b.String()
}

func metaBlock(art Artifact) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-title\" content=\"%s\">", html.EscapeString(art.Title))
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-machine\" content=\"%s\">", html.EscapeString(art.Machine))
	fmt.Fprintf(&b, "\n<meta name=\"mycelium-created\" content=\"%s\">", art.Created.UTC().Format(timeFormat))
	if !art.Expires.IsZero() {
		fmt.Fprintf(&b, "\n<meta name=\"mycelium-expires\" content=\"%s\">", art.Expires.UTC().Format(timeFormat))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

var (
	assetRef  = regexp.MustCompile(`(?i)(?:src|href)="([^"]+)"`)
	schemeRef = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)
)

// ExternalRefs scans raw content for relative links and assets that will not resolve from disk.
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
	for _, m := range mdLink.FindAllSubmatch(raw, -1) {
		ref := strings.TrimSpace(string(m[1]))
		if ref == "" || seen[ref] || strings.HasPrefix(ref, "#") || schemeRef.MatchString(ref) {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}
