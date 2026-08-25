package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

// Opencode is the adapter for OpenCode's config format.
type Opencode struct{}

func init() {
	Register(&Opencode{})
}

func (o *Opencode) Name() string { return "opencode" }

func (o *Opencode) TargetPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
	}
}

// Generate writes the rules to AGENTS.md, each skill to its own SKILL.md, and
// mycelium's MCP server into opencode.json.
//
// OpenCode's declaration is not the mcpServers shape the other assistants
// take: the object is called mcp, the transport is tagged with type "local",
// and command is one argv array with no separate args key. Its schema
// requires exactly those two fields.
func (o *Opencode) Generate(input Input) (*Output, error) {
	out := &Output{Files: make(map[string]string)}
	home, _ := os.UserHomeDir()

	if config, ok := opencodeConfig(home); ok {
		input.MCPTools = declareMCPServer(out, config, "mcp", mcpLocalServer())
	}

	var sections []string

	sections = append(sections, recapReminder)

	sections = append(sections, ruleSections(input)...)

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	agentsMd := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	out.Files[agentsMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		skillPath := filepath.Join(home, ".config", "opencode", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}

// opencodeConfig returns the config file to declare the MCP server in, or
// false when there is none this tool can safely touch.
//
// OpenCode accepts opencode.jsonc too, with the comments and trailing commas
// encoding/json refuses. Writing the .json sibling anyway would leave two
// config files on disk and no way to tell from here which one OpenCode picks,
// so a .jsonc present means mycelium declares nothing and the rules fall back
// to the CLI form.
func opencodeConfig(home string) (string, bool) {
	dir := filepath.Join(home, ".config", "opencode")
	if _, err := os.Stat(filepath.Join(dir, "opencode.jsonc")); err == nil {
		return "", false
	}
	return filepath.Join(dir, "opencode.json"), true
}
