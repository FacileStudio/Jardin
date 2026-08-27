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
	Path    string `json:"path,omitempty" jsonschema:"path to the markdown (.md) or HTML (.html) file to record"`
	Content string `json:"content,omitempty" jsonschema:"inline markdown or HTML content to record as an artifact (alternative to path)"`
	Title   string `json:"title,omitempty" jsonschema:"overrides the document's own title as the artifact's name"`
	Keep    bool   `json:"keep,omitempty" jsonschema:"pin the artifact so it is never swept; the default expires in 30 days"`
}

type publishArtifactOutput struct {
	ID       string   `json:"id" jsonschema:"the artifact's name, used by mycelium artifact open and rm"`
	Path     string   `json:"path" jsonschema:"where the page was written on this machine"`
	URL      string   `json:"url,omitempty" jsonschema:"the canonical web URL of the artifact on the Mycelium server"`
	Expires  string   `json:"expires,omitempty" jsonschema:"when it will be swept; absent when pinned"`
	Opened   bool     `json:"opened" jsonschema:"true when a browser was opened here, false when this machine has no display"`
	Unusable []string `json:"unresolved_refs,omitempty" jsonschema:"relative src or href values that cannot load from disk"`
}

func publishArtifact(_ context.Context, _ *mcp.CallToolRequest, in publishArtifactInput) (*mcp.CallToolResult, publishArtifactOutput, error) {
	path := strings.TrimSpace(in.Path)
	content := strings.TrimSpace(in.Content)
	if path == "" && content == "" {
		return nil, publishArtifactOutput{}, errors.New("publish_artifact needs either path or inline content")
	}
	var art artifacts.Artifact
	var raw []byte
	var err error
	req := artifacts.Request{
		Title:   in.Title,
		Machine: config.MachineName(),
		Pinned:  in.Keep,
	}
	if content != "" {
		raw = []byte(in.Content)
		filename := "inline.md"
		if strings.HasPrefix(content, "<html") || strings.HasPrefix(content, "<!DOCTYPE") {
			filename = "inline.html"
		}
		art, err = artifacts.AddContent(config.DataDir(), raw, filename, req, time.Now())
	} else {
		req.Source = path
		raw, _ = os.ReadFile(path)
		art, err = artifacts.Add(config.DataDir(), req, time.Now())
	}
	if err != nil {
		return nil, publishArtifactOutput{}, err
	}
	cfg, _ := config.LoadMyceliumConfig()
	serverURL := ""
	if cfg != nil {
		serverURL = cfg.ServerURL()
	}
	artURL := artifacts.URL(serverURL, art.ID)
	out := publishArtifactOutput{ID: art.ID, Path: art.Path, URL: artURL}
	if !art.Expires.IsZero() {
		out.Expires = art.Expires.UTC().Format(time.RFC3339)
	}
	if len(raw) > 0 {
		out.Unusable = artifacts.ExternalRefs(raw)
	}
	if artifacts.HasDisplay() {
		target := art.Path
		if artURL != "" {
			target = artURL
		}
		out.Opened = artifacts.Open(target) == nil
	}
	return nil, out, nil
}
