package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

type Codex struct{}

func init() {
	Register(&Codex{})
}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) TargetPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".codex", "AGENTS.md"),
	}
}

func (c *Codex) Generate(input Input) (*Output, error) {
	var sections []string

	for _, rule := range input.Rules {
		sections = append(sections, strings.TrimSpace(rule.Content))
	}

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	out := &Output{Files: make(map[string]string)}

	home, _ := os.UserHomeDir()
	agentsMd := filepath.Join(home, ".codex", "AGENTS.md")
	out.Files[agentsMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		skillPath := filepath.Join(home, ".codex", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}
