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

	for _, skill := range input.Skills {
		sections = append(sections, strings.TrimSpace(skill.Content))
	}

	out := &Output{Files: make(map[string]string)}

	home, _ := os.UserHomeDir()
	agentsMd := filepath.Join(home, ".codex", "AGENTS.md")
	out.Files[agentsMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	return out, nil
}
