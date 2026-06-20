package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

type Claude struct{}

func init() {
	Register(&Claude{})
}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) TargetPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
	}
}

func (c *Claude) Generate(input Input) (*Output, error) {
	var sections []string

	for _, rule := range input.Rules {
		sections = append(sections, strings.TrimSpace(rule.Content))
	}

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	out := &Output{Files: make(map[string]string)}

	home, _ := os.UserHomeDir()
	claudeMd := filepath.Join(home, ".claude", "CLAUDE.md")
	out.Files[claudeMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		cmdPath := filepath.Join(home, ".claude", "commands", skill.Name+".md")
		out.Files[cmdPath] = skill.Content
	}

	return out, nil
}
