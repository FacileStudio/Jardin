package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// searchMemoryTool declares the wiki search.
//
// ReadOnlyHint puts it in the client's read-only group, which a user approves
// once in bulk instead of confirming on every call. OpenWorldHint is false even
// though the search may reach the Mycelium server: the world it reaches is one
// configured host holding this user's own wiki, not the open internet.
func searchMemoryTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "search_memory",
		Title: "Search memory",
		Description: "Search the shared agent wiki, best match first. The Mycelium server's hybrid " +
			"search answers when it can and the local index answers when it cannot, and the result " +
			"says which one did, so a fallback is never silent. Read this before non-trivial work " +
			"rather than rediscovering what is already written down.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: hint(false),
			IdempotentHint:  true,
			OpenWorldHint:   hint(false),
		},
	}
}

// listFlowsTool declares the flow inventory.
//
// It reads two files per flow and changes neither, so it carries the same
// read-only annotations as the search. Trust comes back as a field rather than
// a rendered column: a model deciding whether to call run_flow should read a
// value, not parse a table built for a terminal.
func listFlowsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "list_flows",
		Title: "List flows",
		Description: "List the recorded procedures on this machine with their step count and trust " +
			"state. A flow runs only when a human on this machine has pinned its exact content, so " +
			"trust is what decides whether run_flow executes a flow or refuses it.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: hint(false),
			IdempotentHint:  true,
			OpenWorldHint:   hint(false),
		},
	}
}

// runFlowTool declares the one tool here that executes anything.
//
// It takes a flow name and nothing else. Command injection is the dominant way
// MCP servers are abused, and a fixed set of procedures a human read and pinned
// is this server's whole defence against it; an args or command passthrough
// would hand that back. DestructiveHint and OpenWorldHint are both true because
// a flow's steps are arbitrary shell commands that may reach anything.
//
// Those annotations still only shape a prompt. The spec requires clients to
// treat them as untrusted, so the pin checked in trust.go is the real gate.
func runFlowTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "run_flow",
		Title: "Run a flow",
		Description: "Run a recorded procedure by name and return each step's exit code, output and " +
			"the path of the run record. Takes a flow name and nothing else: no command or argument " +
			"can be passed, and a flow no human has pinned on this machine is refused with the " +
			"command that fixes it.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: hint(true),
			IdempotentHint:  false,
			OpenWorldHint:   hint(true),
		},
	}
}

// publishArtifactTool declares the one tool that produces something for a human
// rather than for the model.
func publishArtifactTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "publish_artifact",
		Title: "Publish an artifact",
		Description: "Record a markdown or HTML document in the synced tree and return the path it " +
			"landed at, so a page produced on a headless machine opens on the machine the human is " +
			"sitting at. It is stored, never hosted: there is no URL and no link to hand anybody.\n\n" +
			"Use it when the answer is structural rather than linear: a comparison across many items, " +
			"a timeline, a graph, a table wider than a terminal. Answer in the conversation first and " +
			"record an artifact as an attachment to that answer, never in place of it. A finding belongs " +
			"in the wiki as text; an artifact is the picture of one, and it expires in 30 days.\n\n" +
			"The page must carry everything it needs inline, because a file opened from disk cannot " +
			"fetch its siblings. The document's title becomes the artifact's name, so recording the " +
			"same title twice replaces it rather than piling up copies.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: hint(false),
			IdempotentHint:  true,
			OpenWorldHint:   hint(false),
		},
	}
}

func publishReportTool() *mcp.Tool {
	t := publishArtifactTool()
	t.Name = "publish_report"
	t.Title = "Publish a report"
	return t
}
