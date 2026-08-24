package memory

import (
	"strings"
)

// Chunk is one retrievable unit of the wiki: a single `### finding` block, or
// the page preamble before the first one. Header carries the context the block
// itself lacks — a block saying "the file is created lazily" means nothing
// without the page it belongs to.
//
// Related is the page's frontmatter `related:` list, comma-joined. It is a
// string and not a slice for the same reason Meta's fields are: Chunk is
// compared with == to decide whether a block changed, and one reference field
// would turn that comparison into a compile error. It is carried per chunk
// rather than looked up per page because the ranker consumes chunks and the
// frontmatter is gone by the time it does.
type Chunk struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
	Line    int    `json:"line"`
	Header  string `json:"header"`
	Body    string `json:"body"`
	Related string `json:"related,omitempty"`
	Meta    Meta   `json:"meta"`
}

// Meta is the optional metadata a finding declares in an HTML comment under its
// heading. Every field is a plain string and there is deliberately no slice or
// map here: Chunk is compared with == to decide whether a block changed, and a
// single reference field would turn that comparison into a compile error.
type Meta struct {
	ID         string `json:"id,omitempty"`
	Date       string `json:"date,omitempty"`
	Source     string `json:"source,omitempty"`
	Confirmed  string `json:"confirmed,omitempty"`
	Supersedes string `json:"supersedes,omitempty"`
}

// Text is what gets indexed and embedded: the header followed by the block,
// so the vector carries the page's identity as well as the block's content,
// with anything the author struck through removed.
//
// A ~~struck-through~~ span is the wiki's way of saying a claim turned out to
// be wrong, and it is normally followed by "[SUPERSEDED by: ...]" and the
// correction in the same finding. Both halves are indexed today, so a query can
// match the dead half and hand an agent a claim the page itself contradicts two
// lines further down. Dropping it from the index is a score change, not a
// deletion: the page still holds every word, a reader still sees it, and
// unstriking the text brings it straight back.
//
// This is the same call takeMeta already makes for the metadata comment. Not
// everything in a body is worth paying for on every query.
func (c Chunk) Text() string {
	body := dropStruck(c.Body)
	if c.Header == "" {
		return body
	}
	return c.Header + "\n\n" + body
}

// dropStruck removes ~~ ... ~~ spans, which run across lines in this corpus and
// so cannot be handled a line at a time.
//
// Text inside backticks is literal in markdown, so a page documenting the marker
// itself writes `~~struck~~` and means the six characters, not a retraction. The
// scan steps over a code span whole. A struck claim that quotes a path or a
// command is still stripped entire, because its opening ~~ is reached first and
// the whole span goes with it, backticks and all.
//
// Anything unterminated, a lone ~~ or an odd backtick, leaves the rest of the
// body untouched. The failure mode here has to be stripping too little: a stray
// pair of tildes must never be able to delete a page from the index.
//
// A stripped span becomes a space so the words either side stay two tokens.
func dropStruck(body string) string {
	var out strings.Builder
	for i := 0; i < len(body); {
		switch {
		case body[i] == '`':
			end := strings.IndexByte(body[i+1:], '`')
			if end < 0 {
				out.WriteString(body[i:])
				return out.String()
			}
			out.WriteString(body[i : i+end+2])
			i += end + 2
		case strings.HasPrefix(body[i:], "~~"):
			end := strings.Index(body[i+2:], "~~")
			if end < 0 {
				out.WriteString(body[i:])
				return out.String()
			}
			out.WriteString(" ")
			i += end + 4
		default:
			out.WriteByte(body[i])
			i++
		}
	}
	return out.String()
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
	front, rest := frontmatter(content)
	lines := strings.Split(rest.text, "\n")
	header := chunkHeader(path, front.title, front.kind)

	var chunks []Chunk
	current := Chunk{Path: path, Line: rest.offset + 1, Header: header, Related: front.related}
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
			Related: front.related,
		}
	}
	return appendChunk(chunks, current, body)
}

func appendChunk(chunks []Chunk, c Chunk, body []string) []Chunk {
	c.Meta, body = takeMeta(body)
	c.Body = strings.TrimSpace(strings.Join(body, "\n"))
	if c.Body == "" && c.Heading == "" {
		return chunks
	}
	if c.Heading != "" {
		c.Header += " / " + c.Heading
	}
	return append(chunks, split(c)...)
}

// takeMeta lifts the metadata comment off the front of a finding's body and
// returns the body without it. The comment counts as metadata only when it
// yields at least one recognised field, because `<!-- lang:fr -->` is a live
// marker in this wiki and stripping it would delete the exemption it grants.
// Stripping matters for the rest: Text() is what gets embedded, and `<!--`
// noise in a vector or a BM25 index is paid for on every query.
func takeMeta(body []string) (Meta, []string) {
	start := 0
	for start < len(body) && strings.TrimSpace(body[start]) == "" {
		start++
	}
	if start == len(body) || !strings.HasPrefix(strings.TrimSpace(body[start]), "<!--") {
		return Meta{}, body
	}
	inner, rest, ok := metaBlock(body, start)
	if !ok {
		return Meta{}, body
	}
	meta := Meta{
		ID:         scalar(inner, "id"),
		Date:       scalar(inner, "date"),
		Source:     scalar(inner, "source"),
		Confirmed:  scalar(inner, "confirmed"),
		Supersedes: scalar(inner, "supersedes"),
	}
	if meta == (Meta{}) {
		return Meta{}, body
	}
	return meta, rest
}

// metaBlock splits a comment block off the front of a body and returns its
// inner text, the lines that follow it and whether it closed at all. Each line
// is trimmed before joining so an indented continuation still reaches scalar()
// looking like a frontmatter line. A blank line ends the search, which is what
// keeps an unterminated `<!--` from swallowing the rest of a page that has no
// `-->` anywhere in it.
func metaBlock(body []string, start int) (string, []string, bool) {
	var inner []string
	for i := start; i < len(body); i++ {
		line := strings.TrimSpace(body[i])
		if i == start {
			line = strings.TrimSpace(strings.TrimPrefix(line, "<!--"))
		} else if line == "" {
			return "", nil, false
		}
		cut := strings.Index(line, "-->")
		if cut < 0 {
			inner = append(inner, line)
			continue
		}
		inner = append(inner, strings.TrimSpace(line[:cut]))
		rest := withTail(strings.TrimSpace(line[cut+len("-->"):]), body[i+1:])
		return strings.Join(inner, "\n"), rest, true
	}
	return "", nil, false
}

// withTail puts whatever was typed after the closing `-->` back at the head of
// the body. Dropping it looked harmless because the page still renders, but the
// sentence would then be missing from every search result that page returns,
// and nothing about the file would show that it had gone.
func withTail(tail string, rest []string) []string {
	if tail == "" {
		return rest
	}
	return append([]string{tail}, rest...)
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
