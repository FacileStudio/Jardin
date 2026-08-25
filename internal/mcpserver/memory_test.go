package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wikiFixture writes one page into a temporary memory root.
func wikiFixture(t *testing.T) {
	t.Helper()
	isolate(t)
	dir := filepath.Join(config.MemoryDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"ranking.md": "---\ntitle: Ranking\n---\n\n### bm25 replaced the strict AND\n" +
			"Strict AND collapsed on reformulated queries.\n",
		"queries.md": "---\ntitle: Queries\n---\n\n### reformulated queries still rank\n" +
			"A reformulated query keeps ranking because bm25 scores every term.\n",
	}
	for name, body := range pages {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// callSearch runs search_memory and decodes the structured half of the answer.
func callSearch(t *testing.T, args map[string]any) (*mcp.CallToolResult, searchOutput) {
	t.Helper()
	res, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search_memory", Arguments: args,
	})
	if err != nil {
		t.Fatalf("search_memory returned a protocol error: %v", err)
	}
	if res.IsError {
		return res, searchOutput{}
	}
	var out searchOutput
	decode(t, res, &out)
	return res, out
}

// The policy cmd/memory.go already follows: a server that cannot answer is a
// fallback, never a failed search. The difference here is that the fallback
// says so, because an agent reading a silent empty answer as "the wiki has
// nothing" is the failure this tool exists to stop.
func TestSearchMemoryFallsBackToTheLocalIndexAndSaysWhy(t *testing.T) {
	wikiFixture(t)
	t.Setenv(config.VectorSearchEnv, "true")
	t.Setenv(config.URLEnv, "http://127.0.0.1:1")
	t.Setenv(config.TokenEnv, "tok")

	_, out := callSearch(t, map[string]any{"query": "bm25 ranking"})
	if len(out.Results) == 0 {
		t.Fatal("search_memory returned nothing, want the local index to answer")
	}
	if out.Results[0].Path != "ranking.md" {
		t.Errorf("Results[0].Path = %q, want ranking.md", out.Results[0].Path)
	}
	if out.Source != sourceLocal {
		t.Errorf("Source = %q, want %q", out.Source, sourceLocal)
	}
	if !strings.Contains(out.Note, "the server did not answer") {
		t.Errorf("Note = %q, want it to say why the server was skipped", out.Note)
	}
}

// An empty query would rank every page equally and return noise, so it is
// refused as a tool execution error the model can correct on its own.
func TestSearchMemoryRefusesAnEmptyQuery(t *testing.T) {
	wikiFixture(t)

	res, _ := callSearch(t, map[string]any{"query": "   "})
	if !res.IsError {
		t.Fatal("search_memory accepted a blank query")
	}
	if !strings.Contains(resultText(res), "needs a query") {
		t.Errorf("refusal = %q, want it to say a query is missing", resultText(res))
	}
}

// Limit is the model's budget, and honouring it matters most on the local half,
// which ranks every line in the wiki and would otherwise return all of them.
func TestSearchMemoryHonoursTheRequestedLimit(t *testing.T) {
	wikiFixture(t)

	const query = "reformulated queries bm25"
	if _, unlimited := callSearch(t, map[string]any{"query": query}); len(unlimited.Results) < 2 {
		t.Fatalf("the fixture yields %d results unlimited, so a limit of 1 would prove nothing",
			len(unlimited.Results))
	}

	_, out := callSearch(t, map[string]any{"query": query, "limit": 1})
	if len(out.Results) != 1 {
		t.Errorf("got %d results, want 1", len(out.Results))
	}
}
