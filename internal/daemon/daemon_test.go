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

func TestStablePathPrefersPathEntryOverVersionedDir(t *testing.T) {
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "jardin", "1.2.3", "bin")
	if err := os.MkdirAll(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(cellar, "jardin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stableDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(stableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(stableDir, "jardin")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(bin)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", stableDir)
	if got := stablePath(resolved); got != link {
		t.Fatalf("expected the stable symlink %s, got the versioned path %s", link, got)
	}

	t.Setenv("PATH", filepath.Join(dir, "empty"))
	if got := stablePath(resolved); got != resolved {
		t.Fatalf("expected fallback to %s, got %s", resolved, got)
	}
}
