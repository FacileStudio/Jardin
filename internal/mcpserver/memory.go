package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// searchTimeout matches cmd/memory.go. A server that has not answered in
	// three seconds is a server the local index should be answering for.
	searchTimeout = 3 * time.Second
	// defaultSearchLimit matches the --limit default of "mycelium memory search",
	// so the two surfaces return the same thing for the same query.
	defaultSearchLimit = 20

	sourceServer = "server"
	sourceLocal  = "local index"
)

// searchInput is what a model may ask for. Query is the only required field.
type searchInput struct {
	Query string `json:"query" jsonschema:"the words to look for across the wiki"`
	Limit int    `json:"limit,omitempty" jsonschema:"how many results to return at most; 0 means 20"`
}

// searchHit is one matching line, carrying enough to open the page it came from.
type searchHit struct {
	Path                 string `json:"path" jsonschema:"page path relative to the memory root"`
	Line                 int    `json:"line" jsonschema:"line number of the match within that page"`
	Content              string `json:"content" jsonschema:"the matching line"`
	Score                int    `json:"score" jsonschema:"relevance, higher is better"`
	Date                 string `json:"date,omitempty" jsonschema:"the finding's date when it has one"`
	ChangedSinceRatified bool   `json:"changed_since_ratified" jsonschema:"true when a normative page changed after a human last ratified it, which makes this result non-authoritative"`
}

// searchOutput reports the hits and which index produced them.
type searchOutput struct {
	Results []searchHit `json:"results"`
	Source  string      `json:"source" jsonschema:"which index answered: server or local index"`
	Note    string      `json:"note,omitempty" jsonschema:"why the server was skipped, or what else degraded this answer"`
}

// searchMemory answers from the server when it can and from the local index
// otherwise, which is cmd/memory.go's policy unchanged: a remote failure is
// demoted to a fallback rather than returned, because a search that errors
// because a machine somewhere is down is worse than one ranked lexically.
//
// The reason the server was skipped travels back in Note. A silent fallback is
// how an agent reads a 401 as an empty wiki and carries on regardless, which is
// the failure that put this package here.
func searchMemory(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, searchOutput{}, errors.New("search_memory needs a query")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	source, notes := sourceServer, []string(nil)
	results, degraded, err := searchServer(ctx, query, limit)
	if err != nil {
		source = sourceLocal
		notes = append(notes, "the server did not answer, so this is the local index: "+err.Error())
		if results, err = memory.SearchChunks(config.MemoryDir(), query); err != nil {
			return nil, searchOutput{}, err
		}
	}
	if degraded {
		notes = append(notes, "the server ranked lexically: its vector half is unavailable")
	}
	if len(results) > limit {
		results = results[:limit]
	}
	changed, err := memory.ChangedPages(config.DataDir())
	if err != nil {
		notes = append(notes, "ratification state is unknown, so no result is marked: "+err.Error())
	}
	return nil, searchOutput{Results: hits(results, changed), Source: source, Note: strings.Join(notes, "; ")}, nil
}

// searchServer puts the query to this machine's configured Mycelium server. It
// refuses before making a request in the three cases that cannot succeed, so
// the caller falls back at once instead of after a timeout.
func searchServer(ctx context.Context, query string, limit int) ([]memory.SearchResult, bool, error) {
	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		return nil, false, err
	}
	if !cfg.SemanticEnabled() {
		return nil, false, errors.New("vector search is off (set vector_search: true in ~/.mycelium.yml)")
	}
	if cfg.ServerURL() == "" || cfg.AuthToken() == "" {
		return nil, false, errors.New("no server configured")
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	return memory.SearchRemote(ctx, memory.RemoteSearch{
		BaseURL: cfg.ServerURL(), Token: cfg.AuthToken(), SpaceID: cfg.Space,
		Query: query, Limit: limit,
	})
}

// hits converts search results and marks any page whose normative content
// changed since a human ratified it. That marker is not decoration: a page in
// that state is not authoritative, which is why the CLI prints it beside every
// result and why it belongs in the structured answer too.
func hits(results []memory.SearchResult, changed map[string]bool) []searchHit {
	out := make([]searchHit, 0, len(results))
	for _, r := range results {
		out = append(out, searchHit{
			Path: r.Path, Line: r.Line, Content: r.Content, Score: r.Score,
			Date: r.Date, ChangedSinceRatified: changed[r.Path],
		})
	}
	return out
}
