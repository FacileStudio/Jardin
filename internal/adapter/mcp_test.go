package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mcpFixture writes existing content to a temp file and returns its path. An
// empty string means the file is absent, which is the first-install case.
func mcpFixture(t *testing.T, existing string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if existing != "" {
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// serversIn reads the mcpServers object out of merged content.
func serversIn(t *testing.T, content, key string) map[string]any {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal([]byte(content), &settings); err != nil {
		t.Fatalf("merged content is not JSON: %v\n%s", err, content)
	}
	servers, ok := settings[key].(map[string]any)
	if !ok {
		t.Fatalf("no %q object in:\n%s", key, content)
	}
	return servers
}

// TestTheDeclarationNamesTheBinaryAndTheStdioSubcommand is the shape check: a
// client that reads this file must end up spawning `<binary> mcp`.
func TestTheDeclarationNamesTheBinaryAndTheStdioSubcommand(t *testing.T) {
	content, _, ok := mergeMCPServers(mcpFixture(t, ""), "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("expected a declaration for a fresh install")
	}
	entry, isMap := serversIn(t, content, "mcpServers")[mcpServerName].(map[string]any)
	if !isMap {
		t.Fatalf("no %q server in:\n%s", mcpServerName, content)
	}
	if command, _ := entry["command"].(string); command == "" {
		t.Errorf("entry has no command: %v", entry)
	}
	args, _ := entry["args"].([]any)
	if len(args) != 1 || args[0] != mcpSubcommand {
		t.Errorf("args = %v, want [%q]", args, mcpSubcommand)
	}
}

// TestASecondMergeIsByteIdenticalToTheFirst is the property `mycelium diff`
// depends on. Generate reads what the last install wrote, so anything
// non-deterministic here — map order, a re-encoded number, a missing trailing
// newline — shows up forever as an agent that is permanently out of date.
func TestASecondMergeIsByteIdenticalToTheFirst(t *testing.T) {
	path := mcpFixture(t, `{"numTurns":3,"mcpServers":{"figma":{"command":"figma-mcp"}}}`)

	first, _, ok := mergeMCPServers(path, "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("first merge produced nothing")
	}
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}

	second, _, ok := mergeMCPServers(path, "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("second merge produced nothing")
	}
	if second != first {
		t.Errorf("second merge differs from the first:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// TestAStaleEntryUnderTheOldNameIsReplacedNotJoined is the bug this merge
// exists for. The 2026-08 rename left a jardin entry beside the mycelium one,
// pointing at a binary that had been deleted, and diff reported no changes
// because the key it wanted was already present.
func TestAStaleEntryUnderTheOldNameIsReplacedNotJoined(t *testing.T) {
	existing := `{"mcpServers":{
		"jardin":{"command":"/home/yann/.local/bin/jardin","args":["mcp"]},
		"figma":{"command":"figma-mcp","args":["--stdio"]}
	}}`
	content, _, ok := mergeMCPServers(mcpFixture(t, existing), "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("merge produced nothing")
	}

	servers := serversIn(t, content, "mcpServers")
	if _, stale := servers["jardin"]; stale {
		t.Errorf("stale entry under the old name survived:\n%s", content)
	}
	if _, ours := servers[mcpServerName]; !ours {
		t.Errorf("no %q entry after the merge:\n%s", mcpServerName, content)
	}
	if len(servers) != 2 {
		t.Errorf("expected exactly the foreign server and ours, got %d: %v", len(servers), servers)
	}
}

// TestAnEntryRenamedByHandIsStillOurs covers the other half of the same bug:
// the stale entry is recognised by the binary it spawns, not only by its key,
// because whoever renamed it did not change what it runs.
func TestAnEntryRenamedByHandIsStillOurs(t *testing.T) {
	existing := `{"mcpServers":{"memory":{"command":"/opt/old/mycelium","args":["mcp"]}}}`
	content, _, ok := mergeMCPServers(mcpFixture(t, existing), "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("merge produced nothing")
	}
	if _, stale := serversIn(t, content, "mcpServers")["memory"]; stale {
		t.Errorf("a renamed mycelium entry survived:\n%s", content)
	}
}

// TestForeignServersAndUnknownKeysSurviveTheMerge is why this reads the file
// instead of writing a fresh one: install overwrites whatever it names, and
// these files hold servers and settings mycelium did not put there.
func TestForeignServersAndUnknownKeysSurviveTheMerge(t *testing.T) {
	existing := `{"theme":"dark","mcpServers":{"figma":{"command":"figma-mcp","args":["--stdio"]}}}`
	content, _, ok := mergeMCPServers(mcpFixture(t, existing), "mcpServers", mcpStdioServer())
	if !ok {
		t.Fatal("merge produced nothing")
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(content), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["theme"] != "dark" {
		t.Errorf("unknown top-level key lost:\n%s", content)
	}
	figma, _ := serversIn(t, content, "mcpServers")["figma"].(map[string]any)
	if figma == nil || figma["command"] != "figma-mcp" {
		t.Errorf("foreign server lost or rewritten:\n%s", content)
	}
}

// TestAnUnreadableConfigIsLeftAloneAndDowngradesTheAgent matches InstallHooks
// on corrupt JSON, and adds the consequence the fences depend on: an agent
// whose config could not be parsed did not get the tools this run, so its
// rules must still tell it to use the CLI.
func TestAnUnreadableConfigIsLeftAloneAndDowngradesTheAgent(t *testing.T) {
	path := mcpFixture(t, "{not json")
	if content, _, ok := mergeMCPServers(path, "mcpServers", mcpStdioServer()); ok {
		t.Fatalf("expected a refusal on corrupt JSON, got:\n%s", content)
	}

	out := &Output{Files: make(map[string]string)}
	if declareMCPServer(out, path, "mcpServers", mcpStdioServer()) {
		t.Error("declareMCPServer claimed tool support for an unparseable config")
	}
	if len(out.Files) != 0 {
		t.Errorf("corrupt config queued for overwrite: %v", out.Files)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Errorf("corrupt config was rewritten: %q", data)
	}
}

func TestMCPHealthNamesTheConfigItRefusedToTouch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	msg, ok := MCPHealth([]string{"claude"})
	if ok {
		t.Error("a config mycelium refuses to touch has to fail the check, not pass it quietly")
	}
	for _, want := range []string{"claude", ".claude.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not name %q: %s", want, msg)
		}
	}
}

func TestMCPHealthSeparatesDeclaredFromNotYetInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	declared := `{"mcpServers":{"mycelium":{"command":"/usr/local/bin/mycelium","args":["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(declared), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gemini", "settings.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	msg, ok := MCPHealth([]string{"claude", "gemini", "codex"})
	if !ok {
		t.Fatalf("check failed with nothing wrong: %s", msg)
	}
	if !strings.Contains(msg, "declared for claude") {
		t.Errorf("a declared assistant is not reported as declared: %s", msg)
	}
	if !strings.Contains(msg, "pending for gemini") {
		t.Errorf("an assistant install has not reached is not reported as pending: %s", msg)
	}
	if strings.Contains(msg, "codex") {
		t.Errorf("codex declares nothing by design and must not appear at all: %s", msg)
	}
}
