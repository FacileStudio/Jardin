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

	written, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written == "" {
		t.Fatal("expected settings path")
	}
	settings := readSettings(t, home)
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatal("SessionStart hook missing")
	}
}

func TestInstallHooksPreservesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	existing := `{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}]}}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644)

	if _, err := (&Claude{}).InstallHooks(); err != nil {
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

	if _, err := (&Claude{}).InstallHooks(); err != nil {
		t.Fatal(err)
	}
	written, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if written != "" {
		t.Fatal("second install must be a no-op")
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

	written, err := (&Claude{}).InstallHooks()
	if err != nil || written != "" {
		t.Fatalf("corrupt settings must be skipped, got written=%q err=%v", written, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(data), "{not json") {
		t.Fatal("corrupt file must not be overwritten")
	}
}
