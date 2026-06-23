package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)
	os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)
	os.WriteFile(filepath.Join(home, ".codex", "AGENTS.md"), []byte("x"), 0644)

	got := DetectAgents()
	if len(got) != 2 || got[0] != "claude" || got[1] != "codex" {
		t.Fatalf("expected [claude codex], got %v", got)
	}
}

func TestPlistContent(t *testing.T) {
	p := PlistContent("/usr/local/bin/ruche")
	for _, want := range []string{Label, "<string>/usr/local/bin/ruche</string>", "<string>daemon</string>", "<string>run</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestSystemdContent(t *testing.T) {
	if !strings.Contains(ServiceContent("/x/ruche"), "ExecStart=/x/ruche daemon run") {
		t.Error("service missing ExecStart")
	}
	if !strings.Contains(TimerContent(), "OnUnitActiveSec=300sec") {
		t.Error("timer missing interval")
	}
}
