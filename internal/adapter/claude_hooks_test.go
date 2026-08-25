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
	if strings.Join(added, ", ") != "SessionStart recap hook, status line, MCP server" {
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
	if strings.Join(added, ", ") != "SessionStart recap hook, MCP server" {
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
	if strings.Join(added, ", ") != "status line, MCP server" {
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

// TestInstallHooksLeavesCorruptFileAlone pins the refusal per file, not per
// run: unreadable settings.json must survive, and must not take the MCP
// declaration in the separate ~/.claude.json down with it.
func TestInstallHooksLeavesCorruptFileAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644)

	_, added, err := (&Claude{}).InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(added, ", ") != "MCP server" {
		t.Fatalf("corrupt settings must be skipped, got added=%q", added)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(data), "{not json") {
		t.Fatal("corrupt file must not be overwritten")
	}
}

// TestTheAccountRecordIsNeverLeftTruncated pins the property that made this
// write worth changing. os.WriteFile opens with O_TRUNC, so a crash between the
// open and the last byte leaves ~/.claude.json empty, and it holds the account
// record and every project's history. Writing through a temp file and renaming
// means a reader sees either the old file or the new one, never nothing.
func TestTheAccountRecordIsNeverLeftTruncated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	if err := os.WriteFile(path, []byte(`{"numStartups":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeConfigAtomically(path, `{"numStartups":2}`); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"numStartups":2}` {
		t.Errorf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600: this file carries the account record", info.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "claude.json.*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}
