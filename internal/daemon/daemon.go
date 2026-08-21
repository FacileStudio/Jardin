package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/usage"
)

const Label = "studio.facile.mycelium-sync"

// IntervalSeconds paces the cheap work — scanning sessions and syncing — so
// liveness stays roughly a minute fresh. Regenerating agent configs is far
// more write-heavy, so installRefresh keeps it on its original cadence.
const IntervalSeconds = 60

const installRefresh = 5 * time.Minute

func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, nil
	}
	return stablePath(resolved), nil
}

// stablePath returns a launcher path for resolved that survives upgrades.
// Package managers install into versioned directories — Homebrew's
// Cellar/<formula>/<version> — and expose a stable symlink on PATH. Baking the
// versioned path into a plist or a systemd unit leaves the daemon pointing at a
// binary that disappears on the next upgrade, which fails silently.
func stablePath(resolved string) string {
	onPath, err := exec.LookPath(filepath.Base(resolved))
	if err != nil {
		return resolved
	}
	abs, err := filepath.Abs(onPath)
	if err != nil {
		return resolved
	}
	if target, err := filepath.EvalSymlinks(abs); err != nil || target != resolved {
		return resolved
	}
	return abs
}

func logPath() string {
	return filepath.Join(config.DataDir(), "daemon.log")
}

// Run starts the background sync loop as the current process: it enables the
// usage status line and blocks until interrupted.
func Run() error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	exec.Command(self, "sessions", "scan").Run()
	refreshUsage(self)
	var syncErr error
	if out, err := exec.Command(self, "sync").CombinedOutput(); err != nil {
		syncErr = fmt.Errorf("sync failed: %v: %s", err, out)
	}
	if !installDue(time.Now()) {
		return syncErr
	}
	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		return err
	}
	agents := cfg.Agents
	if len(agents) == 0 {
		agents = DetectAgents()
	}
	for _, agent := range agents {
		if out, err := exec.Command(self, "install", agent).CombinedOutput(); err != nil {
			return fmt.Errorf("install %s failed: %v: %s", agent, err, out)
		}
	}
	markInstalled(time.Now())
	return syncErr
}

// refreshUsage cross-checks subscription limits on machines that opted into a
// usage token. Without one this is a no-op: the numbers already arrive from
// Claude Code's status line, and the endpoint rate-limits hard enough that
// polling it unasked would be rude. FetchOAuth's cache keeps the real request
// rate to once every OAuthCacheTTL regardless of the tick.
func refreshUsage(self string) {
	cfg, err := config.LoadMyceliumConfig()
	if err != nil {
		return
	}
	if usage.ResolveToken(cfg.UsageToken) == "" {
		return
	}
	exec.Command(self, "usage", "--live").Run()
}

func installStampPath() string {
	return filepath.Join(config.DataDir(), ".last-install")
}

func installDue(now time.Time) bool {
	info, err := os.Stat(installStampPath())
	if err != nil {
		return true
	}
	return now.Sub(info.ModTime()) >= installRefresh
}

func markInstalled(now time.Time) {
	path := installStampPath()
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.Close()
	}
	os.Chtimes(path, now, now)
}

// DetectAgents reports the agents whose home-level config directory exists.
//
// Only home-scoped adapters can be found this way. Cursor writes .cursor/rules/
// and Copilot writes .github/copilot-instructions.md, both relative to a
// project, so there is nothing under $HOME to look for and neither belongs
// here. Opencode does have a home config, and its absence was why `doctor`
// reported four agents where `install --all` writes eight.
//
// The `agents` marker reads differently from the rest. ~/.claude exists
// because Claude Code created it, so finding it means that tool is installed;
// ~/.agents is created by this adapter, so finding it means only that someone
// opted into the convention here once. That is deliberate rather than
// circular reasoning: the alternative is keying off whichever consumer
// happens to be installed, and the adapter is named for the specification
// precisely because it does not belong to one. Bootstrap with an explicit
// `mycelium install agents` or any `--all`; the daemon refreshes it from then
// on.
func DetectAgents() []string {
	home, _ := os.UserHomeDir()
	markers := []struct {
		agent string
		path  string
	}{
		{"agents", filepath.Join(home, ".agents")},
		{"claude", filepath.Join(home, ".claude")},
		{"codex", filepath.Join(home, ".codex")},
		{"gemini", filepath.Join(home, ".gemini")},
		{"hermes", filepath.Join(home, "SOUL.md")},
		{"opencode", filepath.Join(home, ".config", "opencode")},
	}
	var found []string
	for _, m := range markers {
		if _, err := os.Stat(m.path); err == nil {
			found = append(found, m.agent)
		}
	}
	return found
}
