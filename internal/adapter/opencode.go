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

func (o *Opencode) Generate(input Input) (*Output, error) {
	var sections []string

	sections = append(sections, recapReminder)

	for _, rule := range input.Rules {
		sections = append(sections, strings.TrimSpace(rule.Content))
	}

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	out := &Output{Files: make(map[string]string)}

	home, _ := os.UserHomeDir()
	agentsMd := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	out.Files[agentsMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		skillPath := filepath.Join(home, ".config", "opencode", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}
