package memory

import (
	"fmt"
	"path/filepath"
	"strings"
)

// bumpUpdated rewrites the page's `updated:` stamp, adding the field when the
// header lacks it. A page with no frontmatter is refused rather than repaired:
// the header carries what retrieval and ratification read, and inventing one
// files a finding on a page the wiki only half sees.
func bumpUpdated(content, day string) (string, error) {
	const fence = "---\n"
	if !strings.HasPrefix(content, fence) {
		return "", fmt.Errorf("the page has no YAML frontmatter, so there is no `updated:` to stamp")
	}
	end := strings.Index(content[len(fence):], "\n---")
	if end < 0 {
		return "", fmt.Errorf("the page's YAML frontmatter is never closed")
	}
	head := strings.Split(content[len(fence):len(fence)+end], "\n")
	rest := content[len(fence)+end:]
	for i, line := range head {
		if strings.HasPrefix(line, "updated:") {
			head[i] = "updated: " + day
			return fence + strings.Join(head, "\n") + rest, nil
		}
	}
	return fence + strings.Join(append(head, "updated: "+day), "\n") + rest, nil
}

// indexPointer returns index.md with a pointer to the page, and whether it
// added one. A page the index already names keeps the line it has: the index is
// a router with one line per page, and a line per finding is the rot itself.
//
// The link text is the page's own `title:`, falling back to its slug for a page
// whose header carries none. Every hand-written line in the corpus reads as a
// title, so a slug among them looks like a different kind of entry and stops
// being scannable, which is the one job the index has.
//
// The hook is the finding being filed. For a page reaching the index for the
// first time that is the whole of what it holds, and it is the only sentence
// about this page anyone has written.
func indexPointer(index, page, pageTitle, finding string) (string, bool) {
	if strings.Contains(index, "("+page+")") {
		return index, false
	}
	name := strings.TrimSpace(pageTitle)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(page), ".md")
	}
	line := fmt.Sprintf("- [%s](%s): %s", name, page, strings.TrimSpace(finding))
	section := "Pages"
	if dir, _, found := strings.Cut(page, "/"); found && dir != "" {
		section = strings.ToUpper(dir[:1]) + dir[1:]
	}
	return insertInSection(index, section, line), true
}

// insertInSection puts the line at the end of its section's entries, starting
// that section at the end of the file when the index has none. Trailing blank
// lines stay below the insertion point, so the spacing between sections lives.
func insertInSection(index, section, line string) string {
	head := "## " + section
	if strings.TrimSpace(index) == "" {
		return strings.Join([]string{head, "", line}, "\n") + "\n"
	}
	lines := strings.Split(strings.TrimRight(index, "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == head {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return strings.Join(append(lines, "", head, "", line), "\n") + "\n"
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := append(append([]string{}, lines[:end]...), line)
	return strings.Join(append(out, lines[end:]...), "\n") + "\n"
}

// appendLog adds the history line. log.md is append-only by invariant, so this
// only grows the file.
func appendLog(content, day, description string) string {
	line := fmt.Sprintf("## [%s] %s | %s\n", day, logOperation, strings.TrimSpace(description))
	if strings.TrimSpace(content) == "" {
		return line
	}
	return strings.TrimRight(content, "\n") + "\n" + line
}
