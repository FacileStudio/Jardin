package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect wires a client to the server over in-memory transports, which
// exercises the same code path stdio does without touching a real stdin.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverSide, clientSide := mcp.NewInMemoryTransports()
	ss, err := New("test").Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// listTools returns what tools/list reports, in the order it reported it.
func listTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	res, err := connect(t).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	return res.Tools
}

// decode reads a tool result's structuredContent into v, which is the half of
// the answer a model is meant to act on.
func decode(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	if res.StructuredContent == nil {
		t.Fatal("the result carries no structuredContent")
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("re-encoding structuredContent: %v", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decoding structuredContent %s: %v", data, err)
	}
}

// resultText joins a result's text content, which is where a tool execution
// error's message lands.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

// Five built-in tools plus one tool per recorded flow. The spec asks for
// stable ordering so a client can cache the list, and a new built-in tool
// appearing here should be a decision somebody made rather than a surprise.
func TestToolsListReportsAllBuiltInAndFlowTools(t *testing.T) {
	tools := listTools(t)
	builtIn := map[string]bool{"list_flows": true, "publish_artifact": true, "publish_report": true, "run_flow": true, "search_memory": true}
	seen := make(map[string]bool)
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = true
		_, ok := builtIn[tool.Name]
		if !ok && !strings.HasPrefix(tool.Name, "run_flow_") {
			t.Errorf("unexpected tool %q", tool.Name)
		}
	}
	for name := range builtIn {
		if !seen[name] {
			t.Errorf("missing built-in tool %q", name)
		}
	}
}

// Annotations drive the client's permission UI: the two read-only tools land in
// the group a user bulk-approves once, and run_flow does not. Every field is
// pinned here because three of the four default to the permissive value when
// left unset, so "we forgot" and "we meant it" look identical on the wire.
func TestEveryToolSetsAllFourAnnotationsExplicitly(t *testing.T) {
	want := map[string]*mcp.ToolAnnotations{
		"search_memory":    {ReadOnlyHint: true, DestructiveHint: hint(false), IdempotentHint: true, OpenWorldHint: hint(false)},
		"list_flows":       {ReadOnlyHint: true, DestructiveHint: hint(false), IdempotentHint: true, OpenWorldHint: hint(false)},
		"run_flow":         {ReadOnlyHint: false, DestructiveHint: hint(true), IdempotentHint: false, OpenWorldHint: hint(true)},
		"publish_artifact": {ReadOnlyHint: false, DestructiveHint: hint(false), IdempotentHint: true, OpenWorldHint: hint(false)},
		"publish_report":   {ReadOnlyHint: false, DestructiveHint: hint(false), IdempotentHint: true, OpenWorldHint: hint(false)},
	}
	// Flow tools inherit the same annotations as run_flow (destructive, open-world)
	flowAnnotations := &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: hint(true), IdempotentHint: false, OpenWorldHint: hint(true)}
	for _, tool := range listTools(t) {
		if tool.Annotations == nil {
			t.Errorf("%s carries no annotations", tool.Name)
			continue
		}
		expected, ok := want[tool.Name]
		if !ok {
			// Flow tool — uses run_flow annotations
			expected = flowAnnotations
		}
		if !reflect.DeepEqual(tool.Annotations, expected) {
			t.Errorf("%s annotations = %+v, want %+v", tool.Name, tool.Annotations, expected)
		}
	}
}

// A tool with no input schema cannot be called, and one with no output schema
// hands the model prose to re-parse instead of data. Both are declared by the
// argument and result types, so this catches a type that stopped generating one.
func TestEveryToolAdvertisesAnObjectInputAndOutputSchema(t *testing.T) {
	for _, tool := range listTools(t) {
		for label, schema := range map[string]any{"inputSchema": tool.InputSchema, "outputSchema": tool.OutputSchema} {
			if schema == nil {
				t.Errorf("%s has no %s", tool.Name, label)
				continue
			}
			object, ok := schema.(map[string]any)
			if !ok || object["type"] != "object" {
				t.Errorf("%s %s = %v, want a JSON schema of type object", tool.Name, label, schema)
			}
		}
	}
}

// TestPublishArtifactWithInlineContent points MYCELIUM_URL at a port nothing
// listens on, and that is the point: publish_artifact syncs what it records,
// and LoadMyceliumConfig reads ~/.mycelium.yml rather than DATA_DIR, so the
// token is the developer's own. Naming a real server here published a "# Test"
// artifact into the production store on every `go test ./...`, three of them
// on 2026-08-29 before anyone noticed. The URL is built from the configured
// server whether the sync reaches it or not, so the assertion holds offline.
func TestPublishArtifactWithInlineContent(t *testing.T) {
	client := connect(t)
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("MYCELIUM_URL", "http://127.0.0.1:1")
	res, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "publish_artifact",
		Arguments: map[string]any{
			"title":   "Inline Artifact",
			"content": "# Test\n\nInline content.",
		},
	})
	if err != nil {
		t.Fatalf("call publish_artifact: %v", err)
	}
	var out publishArtifactOutput
	decode(t, res, &out)
	if out.ID == "" {
		t.Fatal("empty artifact ID")
	}
	if !strings.HasPrefix(out.URL, "http://127.0.0.1:1/artifacts/") {
		t.Fatalf("artifact URL: got %q, want prefix http://127.0.0.1:1/artifacts/", out.URL)
	}
}
