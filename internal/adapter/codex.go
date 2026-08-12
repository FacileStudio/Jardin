package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const recapReminder = "Run `mycelium recap` at the start of the session to see recent work and active claims."

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

	sections = append(sections, recapReminder)

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

// InstallHooks merges a SessionStart hook running `mycelium recap --hook` into
// ~/.codex/hooks.json. The merge is additive: an existing SessionStart hook
// group (e.g. herdr-agent-state.sh) survives untouched, and a second install
// that has nothing left to add writes nothing. Codex SessionStart hooks are
// fire-and-forget on this machine (no hookSpecificOutput channel back into
// the model), so the recap reminder baked into AGENTS.md is the reliable
// fallback; this hook is best-effort.
func (c *Codex) InstallHooks() (string, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(home, ".codex", "hooks.json")

	settings := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", nil, nil
		}
	}

	if !mergeCodexRecapHook(settings) {
		return "", nil, nil
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", nil, err
	}
	return path, []string{"SessionStart recap hook"}, nil
}

func mergeCodexRecapHook(settings map[string]any) bool {
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
				return false
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
	return true
}
