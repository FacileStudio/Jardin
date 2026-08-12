package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

func TestCodexEmitsSkillFilesNotInline(t *testing.T) {
	c := &Codex{}
	out, err := c.Generate(Input{
		Rules:  []cell.NamedFile{{Name: "r", Content: "RULE BODY"}},
		Skills: []cell.NamedFile{{Name: "vhs", Content: "VHS SKILL BODY"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var agents, skill string
	for p, content := range out.Files {
		if strings.HasSuffix(p, filepath.Join(".codex", "AGENTS.md")) {
			agents = content
		}
		if strings.HasSuffix(p, filepath.Join("skills", "vhs", "SKILL.md")) {
			skill = content
		}
	}
	if agents == "" {
		t.Fatal("no AGENTS.md emitted")
	}
	if !strings.Contains(agents, "RULE BODY") {
		t.Error("AGENTS.md missing rules")
	}
	if strings.Contains(agents, "VHS SKILL BODY") {
		t.Error("skill must NOT be inlined into AGENTS.md")
	}
	if skill != "VHS SKILL BODY" {
		t.Errorf("expected skill file content, got %q", skill)
	}
	if !strings.Contains(agents, "mycelium recap") {
		t.Error("AGENTS.md missing recap reminder")
	}
}

func readCodexHooks(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooks map[string]any
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatal(err)
	}
	return hooks
}

func TestCodexInstallHooksCreatesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	written, added, err := (&Codex{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written == "" {
		t.Fatal("expected hooks path")
	}
	if strings.Join(added, ", ") != "SessionStart recap hook" {
		t.Fatalf("added = %q", added)
	}
	settings := readCodexHooks(t, home)
	if _, ok := settings["hooks"].(map[string]any)["SessionStart"]; !ok {
		t.Fatal("SessionStart hook missing")
	}
}

func TestCodexInstallHooksPreservesExistingHerdrHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".codex")
	os.MkdirAll(dir, 0o755)
	existing := `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "command": "bash '/Users/fangafunk/.codex/herdr-agent-state.sh' session",
            "timeout": 10,
            "type": "command"
          }
        ]
      }
    ]
  }
}`
	os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(existing), 0o644)

	written, added, err := (&Codex{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written == "" {
		t.Fatal("expected hooks path")
	}
	if strings.Join(added, ", ") != "SessionStart recap hook" {
		t.Fatalf("added = %q", added)
	}

	settings := readCodexHooks(t, home)
	groups := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(groups) != 2 {
		t.Fatalf("expected 2 SessionStart hook groups, got %d", len(groups))
	}

	var sawHerdr, sawRecap bool
	for _, group := range groups {
		g := group.(map[string]any)
		entries := g["hooks"].([]any)
		for _, entry := range entries {
			e := entry.(map[string]any)
			cmd, _ := e["command"].(string)
			if strings.Contains(cmd, "herdr-agent-state.sh") {
				sawHerdr = true
			}
			if strings.Contains(cmd, "mycelium recap") {
				sawRecap = true
			}
		}
	}
	if !sawHerdr {
		t.Error("existing herdr hook lost")
	}
	if !sawRecap {
		t.Error("recap hook not added")
	}
}

func TestCodexInstallHooksIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, _, err := (&Codex{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	written, added, err := (&Codex{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written != "" || added != nil {
		t.Fatalf("second install must be a no-op, got written=%q added=%q", written, added)
	}
	settings := readCodexHooks(t, home)
	groups := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 hook group, got %d", len(groups))
	}
}

func TestCodexInstallHooksLeavesCorruptFileAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".codex")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "hooks.json"), []byte("{not json"), 0o644)

	written, added, err := (&Codex{}).InstallHooks()
	if err != nil || written != "" || added != nil {
		t.Fatalf("corrupt hooks.json must be skipped, got written=%q added=%q err=%v", written, added, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if !strings.Contains(string(data), "{not json") {
		t.Fatal("corrupt file must not be overwritten")
	}
}
