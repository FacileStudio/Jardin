package memory

import (
	"strings"
)

// remainder is what is left of a page once its frontmatter is removed, with the
// line offset that keeps chunk line numbers true to the file.
type remainder struct {
	text   string
	offset int
}

// pageFront is the three frontmatter fields retrieval reads. It is a struct and
// not three return values because that is four things coming back from one
// function, which is one past what this repo allows.
type pageFront struct {
	title   string
	kind    string
	related string
}

// frontmatter pulls the title, the type and the `related:` list out of a page's
// YAML header and returns what follows, with the line offset so chunk line
// numbers stay true to the file. The related list comes back comma-joined,
// already normalised to bare link names.
func frontmatter(content string) (pageFront, remainder) {
	if !strings.HasPrefix(content, "---\n") {
		return pageFront{}, remainder{text: content}
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return pageFront{}, remainder{text: content}
	}
	head := content[4 : 4+end]
	rest := content[4+end:]
	if cut := strings.Index(rest, "\n"); cut >= 0 {
		rest = rest[cut+1:]
	}
	if cut := strings.Index(rest, "\n"); cut >= 0 && strings.HasPrefix(rest, "-") {
		rest = rest[cut+1:]
	}
	front := pageFront{
		title:   scalar(head, "title"),
		kind:    scalar(head, "type"),
		related: strings.Join(relatedLinks(head), ","),
	}
	return front, remainder{
		text:   rest,
		offset: strings.Count(content[:len(content)-len(rest)], "\n"),
	}
}

func scalar(head, key string) string {
	for _, line := range strings.Split(head, "\n") {
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		return strings.Trim(value, `"'`)
	}
	return ""
}
