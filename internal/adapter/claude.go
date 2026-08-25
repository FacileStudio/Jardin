package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const recapHookCommand = "command -v mycelium >/dev/null 2>&1 && mycelium recap --hook || true"

const statusLineCommand = "mycelium usage --statusline"

// Claude is the adapter for Claude Code's config format.
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
	out := &Output{Files: make(map[string]string)}
	home, _ := os.UserHomeDir()

	input.MCPTools = canDeclareMCP(claudeConfig(home), "mcpServers")

	var sections []string

	sections = append(sections, ruleSections(input)...)

	if input.Machine != "" {
		sections = append(sections, strings.TrimSpace(input.Machine))
	}

	claudeMd := filepath.Join(home, ".claude", "CLAUDE.md")
	out.Files[claudeMd] = strings.Join(sections, "\n\n---\n\n") + "\n"

	for _, skill := range input.Skills {
		skillPath := filepath.Join(home, ".claude", "skills", skill.Name, "SKILL.md")
		out.Files[skillPath] = skill.Content
	}

	return out, nil
}

// claudeConfig is the file Claude Code reads user-scope MCP servers from.
//
// ~/.claude.json, at the top level. Not ~/.claude/settings.json, which holds
// hooks and the status line and no MCP keys at all, and not the projects
// object inside the same file, which is local scope and keyed per directory.
func claudeConfig(home string) string {
	return filepath.Join(home, ".claude.json")
}

// InstallHooks merges a SessionStart hook running `mycelium recap` and a
// status line running `mycelium usage --statusline` into
// ~/.claude/settings.json, and declares the MCP server in ~/.claude.json.
//
// The settings merge is additive: unknown keys, existing hooks and a status
// line the user configured themselves all survive untouched. The MCP merge is
// not, and must not be — it replaces mycelium's own entry under whatever name
// it carries. A second install that has nothing left to change writes
// nothing, and the returned labels name what this run actually did, so the
// caller reports it instead of guessing.
func (c *Claude) InstallHooks() (string, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}

	path, added, err := installClaudeSettings(home)
	if err != nil {
		return "", nil, err
	}

	declared, err := declareClaudeMCP(home)
	if err != nil {
		return "", nil, err
	}
	if declared {
		added = append(added, "MCP server")
		if path == "" {
			path = claudeConfig(home)
		}
	}

	if len(added) == 0 {
		return "", nil, nil
	}
	return path, added, nil
}

// declareClaudeMCP merges mycelium's stdio server into ~/.claude.json and
// reports whether the file changed.
//
// Written here rather than returned in Output.Files, and the asymmetry with
// the gemini and opencode adapters is deliberate. ~/.claude.json is Claude
// Code's live state store — project history, session counters, the account
// record — not a config file. It is 55KB on this machine, rewritten every
// session in JavaScript insertion order, while install overwrites whatever
// Output.Files names and diff line-diffs it against the disk. Emitting it
// there would report every single run as a whole-file reformat that never
// resolves, on top of racing a session that is writing it.
//
// So the file is touched only when the declarations themselves differ, and
// created 0600 rather than 0644 on the chance it is not there yet: it carries
// the account record, and os.WriteFile applies a mode only on creation.
func declareClaudeMCP(home string) (bool, error) {
	path := claudeConfig(home)
	merged, changed, ok := mergeMCPServers(path, "mcpServers", mcpStdioServer())
	if !ok || !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(merged), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// installClaudeSettings merges the recap hook and the status line into
// ~/.claude/settings.json, returning the path only when it wrote.
func installClaudeSettings(home string) (string, []string, error) {
	path := filepath.Join(home, ".claude", "settings.json")

	settings := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", nil, nil
		}
	}

	var added []string
	if mergeRecapHook(settings) {
		added = append(added, "SessionStart recap hook")
	}
	if mergeStatusLine(settings) {
		added = append(added, "status line")
	}
	if len(added) == 0 {
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
	return path, added, nil
}

func mergeRecapHook(settings map[string]any) bool {
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

// mergeStatusLine never replaces a statusLine the user already configured:
// theirs is the prompt they chose to look at all day.
func mergeStatusLine(settings map[string]any) bool {
	if existing, ok := settings["statusLine"]; ok && existing != nil {
		return false
	}
	settings["statusLine"] = map[string]any{
		"type":    "command",
		"command": statusLineCommand,
	}
	return true
}
