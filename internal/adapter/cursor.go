package adapter

import (
	"fmt"
	"strings"
)

type Cursor struct{}

func init() {
	Register(&Cursor{})
}

func (c *Cursor) Name() string { return "cursor" }

func (c *Cursor) TargetPaths() []string {
	return []string{".cursor/rules/"}
}

func (c *Cursor) Generate(input Input) (*Output, error) {
	out := &Output{Files: make(map[string]string)}

	for _, rule := range input.Rules {
		path := fmt.Sprintf(".cursor/rules/%s.mdc", rule.Name)
		content := fmt.Sprintf("---\ndescription: %s\nalwaysApply: true\n---\n\n%s\n",
			rule.Name, strings.TrimSpace(rule.Content))
		out.Files[path] = content
	}

	for _, skill := range input.Skills {
		path := fmt.Sprintf(".cursor/rules/%s.mdc", skill.Name)
		content := fmt.Sprintf("---\ndescription: %s skill\nalwaysApply: false\n---\n\n%s\n",
			skill.Name, strings.TrimSpace(skill.Content))
		out.Files[path] = content
	}

	return out, nil
}
