package memory

import (
	"strings"
)

// Chunk is one retrievable unit of the wiki: a single `### finding` block, or
// the page preamble before the first one. Header carries the context the block
// itself lacks — a block saying "the file is created lazily" means nothing
// without the page it belongs to.
type Chunk struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
	Line    int    `json:"line"`
	Header  string `json:"header"`
	Body    string `json:"body"`
}

// Text is what gets embedded: the header followed by the block, so the vector
// carries the page's identity as well as the block's content.
func (c Chunk) Text() string {
	if c.Header == "" {
		return c.Body
	}
	return c.Header + "\n\n" + c.Body
}

// MaxChunkChars bounds what one chunk carries. Embedding models truncate
// rather than fail: all-minilm keeps 512 tokens and drops the rest silently, so
// a page with no headings — log.md is one chunk of 190k characters — would
// index as its opening paragraph while reporting itself perfectly healthy.
// Roughly four characters per token leaves room for the header on every part.
const MaxChunkChars = 1600

// Chunks splits a page into retrievable blocks. It is a pure function of the
// page bytes: the same file always yields the same chunks, which is what lets
// an index key on a content hash and skip re-embedding unchanged blocks.
func Chunks(path, content string) []Chunk {
	title, kind, rest := frontmatter(content)
	lines := strings.Split(rest.text, "\n")
	header := chunkHeader(path, title, kind)

	var chunks []Chunk
	current := Chunk{Path: path, Line: rest.offset + 1, Header: header}
	var body []string
	for i, line := range lines {
		if !strings.HasPrefix(line, "### ") {
			body = append(body, line)
			continue
		}
		chunks = appendChunk(chunks, current, body)
		body = nil
		current = Chunk{
			Path:    path,
			Heading: strings.TrimSpace(strings.TrimPrefix(line, "###")),
			Line:    rest.offset + i + 1,
			Header:  header,
		}
	}
	return appendChunk(chunks, current, body)
}

func appendChunk(chunks []Chunk, c Chunk, body []string) []Chunk {
	c.Body = strings.TrimSpace(strings.Join(body, "\n"))
	if c.Body == "" && c.Heading == "" {
		return chunks
	}
	if c.Heading != "" {
		c.Header += " / " + c.Heading
	}
	return append(chunks, split(c)...)
}

// split breaks an oversized block at line boundaries so no part is truncated by
// the model. Parts keep the block's heading and header, so each still says
// where it came from, and each carries its own line number so a hit points at
// the right place in the file.
func split(c Chunk) []Chunk {
	if len(c.Text()) <= MaxChunkChars {
		return []Chunk{c}
	}
	var parts []Chunk
	part, lineAt := c, c.Line
	var buf []string
	size := 0
	for i, line := range strings.Split(c.Body, "\n") {
		if size+len(line) > MaxChunkChars && len(buf) > 0 {
			part.Body, part.Line = strings.Join(buf, "\n"), lineAt
			parts = append(parts, part)
			buf, size, lineAt = nil, 0, c.Line+i
		}
		buf = append(buf, line)
		size += len(line) + 1
	}
	if len(buf) > 0 {
		part.Body, part.Line = strings.Join(buf, "\n"), lineAt
		parts = append(parts, part)
	}
	return parts
}

func chunkHeader(path, title, kind string) string {
	parts := []string{strings.TrimSuffix(path, ".md")}
	if title != "" {
		parts = append(parts, title)
	}
	if kind != "" {
		parts = append(parts, kind)
	}
	return strings.Join(parts, " / ")
}

type remainder struct {
	text   string
	offset int
}

// frontmatter pulls the title and type out of a page's YAML header and returns
// what follows, with the line offset so chunk line numbers stay true to the
// file.
func frontmatter(content string) (string, string, remainder) {
	if !strings.HasPrefix(content, "---\n") {
		return "", "", remainder{text: content}
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return "", "", remainder{text: content}
	}
	head := content[4 : 4+end]
	rest := content[4+end:]
	if cut := strings.Index(rest, "\n"); cut >= 0 {
		rest = rest[cut+1:]
	}
	if cut := strings.Index(rest, "\n"); cut >= 0 && strings.HasPrefix(rest, "-") {
		rest = rest[cut+1:]
	}
	return scalar(head, "title"), scalar(head, "type"), remainder{
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
