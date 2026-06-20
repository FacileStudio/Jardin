package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

type Hermes struct{}

func init() {
	Register(&Hermes{})
}

func (h *Hermes) Name() string { return "hermes" }

func (h *Hermes) TargetPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, "SOUL.md"),
	}
}

func (h *Hermes) Generate(input Input) (*Output, error) {
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
	soulMd := filepath.Join(home, "SOUL.md")
	out.Files[soulMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	return out, nil
}
