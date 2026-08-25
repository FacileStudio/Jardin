package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// declaredServers reads the server names an adapter left in a config file.
func declaredServers(t *testing.T, path, key string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no config at %s: %v", path, err)
	}
	return serversIn(t, string(data), key)
}

// TestGeminiDeclaresTheServerInItsSettingsFile is the emit half of the track:
// install writes a config a Gemini session can actually spawn.
func TestGeminiDeclaresTheServerInItsSettingsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := (&Gemini{}).Generate(installInput())
	if err != nil {
		t.Fatal(err)
	}
	content, ok := out.Files[filepath.Join(home, ".gemini", "settings.json")]
	if !ok {
		t.Fatalf("gemini emitted no settings.json: %v", out.Files)
	}
	entry, _ := serversIn(t, content, "mcpServers")[mcpServerName].(map[string]any)
	if entry == nil {
		t.Fatalf("no %q server:\n%s", mcpServerName, content)
	}
	if args, _ := entry["args"].([]any); len(args) != 1 || args[0] != mcpSubcommand {
		t.Errorf("gemini entry does not run the stdio subcommand: %v", entry)
	}
}

// TestOpencodeDeclaresTheServerInItsOwnShape guards the one assistant whose
// format is not mcpServers: the object is mcp, the transport is tagged, and
// command is a single argv array.
func TestOpencodeDeclaresTheServerInItsOwnShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := (&Opencode{}).Generate(installInput())
	if err != nil {
		t.Fatal(err)
	}
	content, ok := out.Files[filepath.Join(home, ".config", "opencode", "opencode.json")]
	if !ok {
		t.Fatalf("opencode emitted no opencode.json: %v", out.Files)
	}
	entry, _ := serversIn(t, content, "mcp")[mcpServerName].(map[string]any)
	if entry == nil {
		t.Fatalf("no %q server:\n%s", mcpServerName, content)
	}
	if entry["type"] != "local" {
		t.Errorf("opencode entry is not tagged local: %v", entry)
	}
	argv, _ := entry["command"].([]any)
	if len(argv) != 2 || argv[1] != mcpSubcommand {
		t.Errorf("opencode command must be one argv array ending in %q, got %v", mcpSubcommand, argv)
	}
	if _, split := entry["args"]; split {
		t.Errorf("opencode takes no args key, got %v", entry)
	}
}

// TestOpencodeDeclaresNothingBesideAJsoncConfig pins the deliberate refusal.
// A .jsonc holds comments encoding/json cannot read, and writing the .json
// sibling would leave two configs with no way to know which one wins.
func TestOpencodeDeclaresNothingBesideAJsoncConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{\n  // mine\n}"), 0o644)

	out, err := (&Opencode{}).Generate(installInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, wrote := out.Files[filepath.Join(dir, "opencode.json")]; wrote {
		t.Error("opencode wrote a competing opencode.json beside a .jsonc")
	}
	agents := out.Files[filepath.Join(dir, "AGENTS.md")]
	if !strings.Contains(agents, "mycelium memory search") {
		t.Error("an agent that got no tools must still be told the command")
	}
}

// TestClaudeDeclaresTheServerInItsUserConfig covers the one adapter whose MCP
// file is written by InstallHooks rather than returned in Output.Files.
func TestClaudeDeclaresTheServerInItsUserConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	servers := declaredServers(t, claudeConfig(home), "mcpServers")
	if _, ok := servers[mcpServerName]; !ok {
		t.Fatalf("no %q server in ~/.claude.json: %v", mcpServerName, servers)
	}
}

// TestClaudeReplacesAStaleEntryRatherThanJoiningIt is the exact incident this
// track exists for, reproduced at the adapter: a jardin entry left by the
// rename, beside which install used to add a second one while diff reported
// no changes.
func TestClaudeReplacesAStaleEntryRatherThanJoiningIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stale := `{"numStartups":41,"mcpServers":{
		"jardin":{"command":"/home/yann/.local/bin/jardin","args":["mcp"]},
		"figma":{"command":"figma-mcp","args":["--stdio"]}
	}}`
	os.WriteFile(claudeConfig(home), []byte(stale), 0o600)

	if _, _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}

	servers := declaredServers(t, claudeConfig(home), "mcpServers")
	if _, stale := servers["jardin"]; stale {
		t.Error("the entry naming the deleted binary survived")
	}
	if _, ours := servers[mcpServerName]; !ours {
		t.Error("mycelium did not declare itself")
	}
	if len(servers) != 2 {
		t.Errorf("expected the foreign server and ours, got %v", servers)
	}
}

// TestASecondClaudeInstallTouchesNothing is the byte-identity half for the
// file Output.Files never sees. ~/.claude.json is Claude Code's live state
// store, so an install that rewrites it when nothing changed is both churn
// and a race against a running session.
func TestASecondClaudeInstallTouchesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(claudeConfig(home))
	if err != nil {
		t.Fatal(err)
	}

	written, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written != "" || added != nil {
		t.Fatalf("second install must be a no-op, got written=%q added=%q", written, added)
	}
	second, err := os.ReadFile(claudeConfig(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Errorf("~/.claude.json changed on a second install:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// TestOnlyConfirmedAssistantsGetADeclaration pins the deliberate skips. codex
// is TOML and there is no TOML decoder in this module; cursor and copilot are
// project-scoped adapters whose MCP paths are user-scoped; ~/.agents
// specifies mcp-settings.json but publishes no schema for it. A wrong config
// file is worse than none, so each of these must keep emitting markdown only.
func TestOnlyConfirmedAssistantsGetADeclaration(t *testing.T) {
	for _, name := range []string{"codex", "cursor", "copilot", "hermes", "agents"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)

			a, err := Get(name)
			if err != nil {
				t.Fatal(err)
			}
			out, err := a.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			for path, content := range out.Files {
				switch filepath.Ext(path) {
				case ".json", ".jsonc", ".toml":
					t.Errorf("%s emitted an unconfirmed config at %s:\n%s", name, path, content)
				}
				if strings.Contains(content, mcpSubcommand+`"`) {
					t.Errorf("%s declared an MCP server at %s:\n%s", name, path, content)
				}
			}
		})
	}
}

// TestEachAgentIsToldAboutTheDoorItActuallyHas ties the two halves together.
// The fence mechanism is worthless if an adapter that declares a server still
// renders the CLI branch, or the reverse — and that mismatch is invisible in
// both files taken on their own.
func TestEachAgentIsToldAboutTheDoorItActuallyHas(t *testing.T) {
	cases := map[string]struct {
		adapter Adapter
		rules   string
		tools   bool
	}{
		"claude":   {&Claude{}, filepath.Join(".claude", "CLAUDE.md"), true},
		"gemini":   {&Gemini{}, filepath.Join(".gemini", "GEMINI.md"), true},
		"opencode": {&Opencode{}, filepath.Join(".config", "opencode", "AGENTS.md"), true},
		"codex":    {&Codex{}, filepath.Join(".codex", "AGENTS.md"), false},
		"agents":   {&Agents{}, filepath.Join(".agents", "AGENTS.md"), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			out, err := tc.adapter.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			rules := out.Files[filepath.Join(home, tc.rules)]
			if rules == "" {
				t.Fatalf("no rules file at %s", tc.rules)
			}

			tool := strings.Contains(rules, "search_memory")
			command := strings.Contains(rules, "mycelium memory search")
			if tool == command {
				t.Fatalf("%s was told both forms or neither: tool=%v command=%v", name, tool, command)
			}
			if tool != tc.tools {
				t.Errorf("%s renders the tool form=%v, want %v", name, tool, tc.tools)
			}
		})
	}
}
