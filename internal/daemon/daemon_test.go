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
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	got := DetectAgents()
	if len(got) != 2 || got[0] != "claude" || got[1] != "codex" {
		t.Fatalf("expected [claude codex], got %v", got)
	}
}

func TestDetectAgentsWithoutGeneratedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	got := DetectAgents()
	if len(got) != 1 || got[0] != "claude" {
		t.Fatalf("agent dir without generated config must still be detected, got %v", got)
	}
}

func TestPlistContent(t *testing.T) {
	p := PlistContent("/usr/local/bin/jardin")
	for _, want := range []string{Label, "<string>/usr/local/bin/jardin</string>", "<string>daemon</string>", "<string>run</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestSystemdContent(t *testing.T) {
	if !strings.Contains(ServiceContent("/x/jardin"), "ExecStart=/x/jardin daemon run") {
		t.Error("service missing ExecStart")
	}
	if !strings.Contains(TimerContent(), "OnUnitActiveSec=300sec") {
		t.Error("timer missing interval")
	}
}
