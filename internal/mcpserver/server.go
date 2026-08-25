// Package mcpserver exposes mycelium's wiki and its recorded flows to an agent
// as MCP tools, spoken over stdio.
//
// Stdio is not a default here, it is the only transport that can work. run_flow
// executes "sh -c" on the calling machine, and the pin deciding whether it may
// lives in ~/.mycelium/.flow-trust.json, which is per machine on purpose. The
// 2026-07-28 spec agrees: a server "intending ... to be run locally SHOULD ...
// use the stdio transport to limit access to just the MCP client".
//
// Every tool declares its annotations explicitly, and every one of them is a
// hint the client may ignore: the spec requires clients to "consider tool
// annotations to be untrusted". They shape a permission prompt and nothing
// else. The gate that actually stops a flow from running is the trust pin,
// checked on every run_flow call in trust.go.
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverName is how a client, and the human reading its permission prompt,
// refer to this server.
const serverName = "mycelium"

// New builds the server with its three tools bound to in-process calls into
// internal/memory and internal/flow.
//
// In process rather than shelling out to the mycelium binary: a subprocess puts
// exit codes, coloured output and a second config load between the model and
// data this package can already reach, and each of those is somewhere the
// answer changes shape on the way back.
func New(version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, nil)
	mcp.AddTool(s, searchMemoryTool(), searchMemory)
	mcp.AddTool(s, listFlowsTool(), listFlows)
	mcp.AddTool(s, runFlowTool(), runFlow)
	return s
}

// Serve runs the server over stdio until the client hangs up or ctx is done.
//
// Nothing reachable from here may write to stdout: it carries the JSON-RPC
// frames, and one stray line desynchronises the stream. That is why runFlow
// leaves flow.Options.Stream nil where "mycelium flow run" mirrors every step
// to os.Stdout.
func Serve(ctx context.Context, version string) error {
	return New(version).Run(ctx, &mcp.StdioTransport{})
}

// hint returns a pointer to a bool, which the pointer-valued annotations need
// so the SDK can tell false from unset.
//
// Both DestructiveHint and OpenWorldHint default to true when absent, so a
// tool that omits them is advertised as destructive and open-world. For the
// two read-only tools that is the exact opposite of the truth, and it costs
// the user the one-click bulk approval that reading this data should get.
func hint(b bool) *bool { return &b }
