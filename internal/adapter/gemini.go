package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

// Gemini is the adapter for Gemini's config format.
type Gemini struct{}

func init() {
	Register(&Gemini{})
}

func (g *Gemini) Name() string { return "gemini" }

func (g *Gemini) TargetPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".gemini", "GEMINI.md"),
	}
}

// Generate writes the rules to GEMINI.md and each skill to its own SKILL.md
// under ~/.gemini/skills/.
//
// Skills were concatenated into GEMINI.md until Gemini CLI adopted the Agent
// Skills standard, and that was the only option at the time. It is now the
// expensive one: every line of every loaded GEMINI.md goes to the model on
// every turn, so inlining charged one full skill body per installed skill,
// permanently, whether or not the task had anything to do with it. On this
// machine that was 76KB of a 98KB file.
//
// Gemini scans ~/.gemini/skills/ (and ~/.agents/skills/ as an alias), injects
// only each skill's name and description at startup, and pulls the body in
// when the model calls activate_skill. Same SKILL.md the claude, codex and
// opencode adapters already write, so nothing here converts.
//
// It also declares mycelium's MCP server in ~/.gemini/settings.json, under
// the top-level mcpServers object the CLI reads. Not the neighbouring mcp
// object: that one holds allow and deny lists of server names, and a server
// definition placed there is silently ignored.
func (g *Gemini) Generate(input Input) (*Output, error) {
	out := &Output{Files: make(map[string]string)}
	home, _ := os.UserHomeDir()

	settings := filepath.Join(home, ".gemini", "settings.json")
	input.MCPTools = declareMCPServer(out, settings, "mcpServers", mcpStdioServer())

	var sections []string

	sections = append(sections, ruleSections(input)...)

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	geminiMd := filepath.Join(home, ".gemini", "GEMINI.md")
	out.Files[geminiMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		skillPath := filepath.Join(home, ".gemini", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}
