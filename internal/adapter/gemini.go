package adapter

import (
	"os"
	"path/filepath"
	"strings"
)

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

func (g *Gemini) Generate(input Input) (*Output, error) {
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
	geminiMd := filepath.Join(home, ".gemini", "GEMINI.md")
	out.Files[geminiMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	return out, nil
}
