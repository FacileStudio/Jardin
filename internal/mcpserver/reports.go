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

type publishReportInput struct {
	Path  string `json:"path" jsonschema:"path to the markdown (.md) or HTML (.html) file to record"`
	Title string `json:"title,omitempty" jsonschema:"overrides the document's own title as the artifact's name"`
	Keep  bool   `json:"keep,omitempty" jsonschema:"pin the artifact so it is never swept; the default expires in 30 days"`
}

type publishReportOutput struct {
	ID       string   `json:"id" jsonschema:"the artifact's name, used by mycelium artifact open and rm"`
	Path     string   `json:"path" jsonschema:"where the page was written on this machine"`
	Expires  string   `json:"expires,omitempty" jsonschema:"when it will be swept; absent when pinned"`
	Opened   bool     `json:"opened" jsonschema:"true when a browser was opened here, false when this machine has no display"`
	Unusable []string `json:"unresolved_refs,omitempty" jsonschema:"relative src or href values that cannot load from disk"`
}

func publishReport(_ context.Context, _ *mcp.CallToolRequest, in publishReportInput) (*mcp.CallToolResult, publishReportOutput, error) {
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return nil, publishReportOutput{}, errors.New("publish_report needs the path of a markdown or HTML file")
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

