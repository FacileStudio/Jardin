package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const recapHookCommand = "command -v mycelium >/dev/null 2>&1 && mycelium recap --hook || true"

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
		skillPath := filepath.Join(home, ".claude", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}

// InstallHooks merges a SessionStart hook running `mycelium recap` into
// ~/.claude/settings.json. The merge is additive: unknown keys and existing
// hooks survive untouched, and a hook already mentioning `mycelium recap` is
// left alone so re-installs are idempotent.
func (c *Claude) InstallHooks() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".claude", "settings.json")

	settings := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", nil
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}
	sessionStart, _ := hooks["SessionStart"].([]any)
	for _, group := range sessionStart {
		g, _ := group.(map[string]any)
		entries, _ := g["hooks"].([]any)
		for _, entry := range entries {
			e, _ := entry.(map[string]any)
			if cmd, _ := e["command"].(string); strings.Contains(cmd, "mycelium recap") {
				return "", nil
			}
		}
	}

	sessionStart = append(sessionStart, map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": recapHookCommand,
			"timeout": 10,
		}},
	})
	hooks["SessionStart"] = sessionStart
	settings["hooks"] = hooks

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
