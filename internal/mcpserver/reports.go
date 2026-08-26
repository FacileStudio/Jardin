package mcpserver

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/reports"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// publishReportInput is the page to record and how long to keep it.
type publishReportInput struct {
	Path  string `json:"path" jsonschema:"path to the self-contained HTML file to record"`
	Title string `json:"title,omitempty" jsonschema:"overrides the document's own <title> as the report's name"`
	Keep  bool   `json:"keep,omitempty" jsonschema:"pin the report so it is never swept; the default expires in 30 days"`
}

// publishReportOutput says where the page landed and what will not resolve.
type publishReportOutput struct {
	ID       string   `json:"id" jsonschema:"the report's name, used by mycelium report open and rm"`
	Path     string   `json:"path" jsonschema:"where the page was written on this machine"`
	Expires  string   `json:"expires,omitempty" jsonschema:"when it will be swept; absent when pinned"`
	Opened   bool     `json:"opened" jsonschema:"true when a browser was opened here, false when this machine has no display"`
	Unusable []string `json:"unresolved_refs,omitempty" jsonschema:"relative src or href values that cannot load from disk"`
}

// publishReport records the page and opens it when somebody is here to look.
//
// The browser is started with a nil Stdout, so the child writes to the null
// device rather than into the JSON-RPC frames this server speaks over stdout.
func publishReport(_ context.Context, _ *mcp.CallToolRequest, in publishReportInput) (*mcp.CallToolResult, publishReportOutput, error) {
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return nil, publishReportOutput{}, errors.New("publish_report needs the path of an HTML file")
	}
	rep, err := reports.Add(config.DataDir(), reports.Request{
		Source:  path,
		Title:   in.Title,
		Machine: config.MachineName(),
		Pinned:  in.Keep,
	}, time.Now())
	if err != nil {
		return nil, publishReportOutput{}, err
	}
	out := publishReportOutput{ID: rep.ID, Path: rep.Path}
	if !rep.Expires.IsZero() {
		out.Expires = rep.Expires.UTC().Format(time.RFC3339)
	}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		out.Unusable = reports.ExternalRefs(raw)
	}
	if reports.HasDisplay() {
		out.Opened = reports.Open(rep.Path) == nil
	}
	return nil, out, nil
}
