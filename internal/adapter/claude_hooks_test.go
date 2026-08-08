package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestInstallHooksCreatesSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	written, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written == "" {
		t.Fatal("expected settings path")
	}
	if strings.Join(added, ", ") != "SessionStart recap hook, status line" {
		t.Fatalf("added = %q", added)
	}
	settings := readSettings(t, home)
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatal("SessionStart hook missing")
	}
	line, ok := settings["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("statusLine missing")
	}
	if line["command"] != statusLineCommand {
		t.Fatalf("statusLine command %v", line["command"])
	}
}

func TestInstallHooksKeepsUserStatusLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	existing := `{"statusLine":{"type":"command","command":"my-own-prompt"}}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644)

	_, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(added, ", ") != "SessionStart recap hook" {
		t.Fatalf("added = %q", added)
	}
	line := readSettings(t, home)["statusLine"].(map[string]any)
	if line["command"] != "my-own-prompt" {
		t.Fatalf("user statusLine clobbered: %v", line["command"])
	}
}

func TestInstallHooksReportsStatusLineOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"mycelium recap --hook"}]}]}}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644)

	written, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written == "" {
		t.Fatal("expected settings path")
	}
	if strings.Join(added, ", ") != "status line" {
		t.Fatalf("added = %q", added)
	}
}

func TestInstallHooksPreservesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	existing := `{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}]}}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644)

	if _, _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	settings := readSettings(t, home)
	if settings["model"] != "opus" {
		t.Fatal("unknown top-level key lost")
	}
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatal("existing hook lost")
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatal("SessionStart not added")
	}
}

func TestInstallHooksIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	written, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written != "" || added != nil {
		t.Fatalf("second install must be a no-op, got written=%q added=%q", written, added)
	}
	settings := readSettings(t, home)
	groups := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 hook group, got %d", len(groups))
	}
}

func TestInstallHooksLeavesCorruptFileAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644)

	written, added, err := (&Claude{}).InstallHooks()
	if err != nil || written != "" || added != nil {
		t.Fatalf("corrupt settings must be skipped, got written=%q added=%q err=%v", written, added, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(data), "{not json") {
		t.Fatal("corrupt file must not be overwritten")
	}
}
