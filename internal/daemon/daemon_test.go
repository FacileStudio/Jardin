package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestDetectAgentsFindsTheAgentsStandardDirectory covers the one marker that
// does not mean what the others mean. ~/.claude exists because Claude Code
// created it, so finding it says that tool is installed; ~/.agents is created
// by mycelium's own agents adapter, so finding it says only that someone opted
// into the convention here once. Detection still has to fire, because it is
// what makes the daemon refresh the tree after the first install.
func TestDetectAgentsFindsTheAgentsStandardDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".agents"), 0755)

	got := DetectAgents()
	if len(got) != 1 || got[0] != "agents" {
		t.Fatalf("expected [agents], got %v", got)
	}
}

// TestDetectAgentsOrderFollowsTheMarkerList pins the ordering the other tests
// in this file assert by index. Creation order is deliberately the reverse of
// the expected result: without this, a reader has no way to tell whether
// got[0] == "claude" is a contract or an accident of how the temp dirs were
// made, and the first person to add a marker breaks those assertions for a
// reason unrelated to what they were testing.
func TestDetectAgentsOrderFollowsTheMarkerList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".codex", ".claude", ".agents"} {
		os.MkdirAll(filepath.Join(home, dir), 0755)
	}

	got := DetectAgents()
	want := []string{"agents", "claude", "codex"}
	if !slices.Equal(got, want) {
		t.Fatalf("DetectAgents() = %v, want %v", got, want)
	}
}

func TestPlistContent(t *testing.T) {
	p := PlistContent("/usr/local/bin/mycelium")
	for _, want := range []string{Label, "<string>/usr/local/bin/mycelium</string>", "<string>daemon</string>", "<string>run</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestSystemdContent(t *testing.T) {
	if !strings.Contains(ServiceContent("/x/mycelium"), "ExecStart=/x/mycelium daemon run") {
		t.Error("service missing ExecStart")
	}
	if !strings.Contains(TimerContent(), fmt.Sprintf("OnUnitActiveSec=%dsec", IntervalSeconds)) {
		t.Error("timer missing interval")
	}
}

func TestStablePathPrefersPathEntryOverVersionedDir(t *testing.T) {
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "mycelium", "1.2.3", "bin")
	if err := os.MkdirAll(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(cellar, "mycelium")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stableDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(stableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(stableDir, "mycelium")
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

func TestInstallDueRespectsRefreshWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)

	now := time.Now()
	if !installDue(now) {
		t.Fatal("first run must install")
	}
	markInstalled(now)
	if installDue(now.Add(time.Minute)) {
		t.Fatal("install must not rerun on every live tick")
	}
	if !installDue(now.Add(installRefresh + time.Second)) {
		t.Fatal("install must resume after the refresh window")
	}
}

func TestRunConsolidationLogsDisabledStage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DATA_DIR", home)
	os.WriteFile(filepath.Join(home, ".mycelium.yml"), []byte("consolidate:\n  enabled: false\n"), 0o600)

	runConsolidation(time.Now())
	data, err := os.ReadFile(logPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "consolidate disabled") {
		t.Fatalf("expected a disabled line in the daemon log, got %q", data)
	}
}

func TestAppendDaemonLogTimestampsAndAppends(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	appendDaemonLog(now, "tick %d", 7)
	appendDaemonLog(now.Add(time.Minute), "again")
	data, err := os.ReadFile(logPath())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "2026-08-26T12:00:00Z tick 7") {
		t.Fatalf("unexpected log lines: %q", lines)
	}
}
