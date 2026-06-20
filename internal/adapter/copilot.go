package adapter

import (
	"strings"
)

type Copilot struct{}

func init() {
	Register(&Copilot{})
}

func (c *Copilot) Name() string { return "copilot" }

func (c *Copilot) TargetPaths() []string {
	return []string{".github/copilot-instructions.md"}
}

func (c *Copilot) Generate(input Input) (*Output, error) {
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
	out.Files[".github/copilot-instructions.md"] = strings.Join(sections, "\n\n---\n\n") + "\n"

	return out, nil
}
