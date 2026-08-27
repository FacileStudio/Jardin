package mcpserver

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/artifacts"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type publishArtifactInput struct {
	Path  string `json:"path" jsonschema:"path to the markdown (.md) or HTML (.html) file to record"`
	Title string `json:"title,omitempty" jsonschema:"overrides the document's own title as the artifact's name"`
	Keep  bool   `json:"keep,omitempty" jsonschema:"pin the artifact so it is never swept; the default expires in 30 days"`
}

type publishArtifactOutput struct {
	ID       string   `json:"id" jsonschema:"the artifact's name, used by mycelium artifact open and rm"`
	Path     string   `json:"path" jsonschema:"where the page was written on this machine"`
	Expires  string   `json:"expires,omitempty" jsonschema:"when it will be swept; absent when pinned"`
	Opened   bool     `json:"opened" jsonschema:"true when a browser was opened here, false when this machine has no display"`
	Unusable []string `json:"unresolved_refs,omitempty" jsonschema:"relative src or href values that cannot load from disk"`
}

func publishArtifact(_ context.Context, _ *mcp.CallToolRequest, in publishArtifactInput) (*mcp.CallToolResult, publishArtifactOutput, error) {
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return nil, publishArtifactOutput{}, errors.New("publish_artifact needs the path of a markdown or HTML file")
	}
	art, err := artifacts.Add(config.DataDir(), artifacts.Request{
		Source:  path,
		Title:   in.Title,
		Machine: config.MachineName(),
		Pinned:  in.Keep,
	}, time.Now())
	if err != nil {
		return nil, publishArtifactOutput{}, err
	}
	out := publishArtifactOutput{ID: art.ID, Path: art.Path}
	if !art.Expires.IsZero() {
		out.Expires = art.Expires.UTC().Format(time.RFC3339)
	}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		out.Unusable = artifacts.ExternalRefs(raw)
	}
	if artifacts.HasDisplay() {
		out.Opened = artifacts.Open(art.Path) == nil
	}
	return nil, out, nil
}
