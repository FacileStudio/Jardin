package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// DetectAgents returns the names of the code assistants whose rule files exist
// on this machine.
// DetectAgents reports the agents whose home-level config directory exists.
//
// Only home-scoped adapters can be found this way. Cursor writes .cursor/rules/
// and Copilot writes .github/copilot-instructions.md, both relative to a
// project, so there is nothing under $HOME to look for and neither belongs
// here. Opencode does have a home config, and its absence was why `doctor`
// reported four agents where `install --all` writes seven.
func DetectAgents() []string {
	home, _ := os.UserHomeDir()
	markers := []struct {
		agent string
		path  string
	}{
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

// Install registers the background service (launchd or systemd) for this user.
func Install() error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(self)
	case "linux":
		return installSystemd(self)
	default:
		return fmt.Errorf("background sync is not supported on %s", runtime.GOOS)
	}
}

// Uninstall removes the background service.
func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return fmt.Errorf("background sync is not supported on %s", runtime.GOOS)
	}
}

// Installed reports whether the background service is registered.
func Installed() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := os.Stat(launchdPath())
		return err == nil
	case "linux":
		_, err := os.Stat(filepath.Join(systemdDir(), "mycelium-sync.timer"))
		return err == nil
	default:
		return false
	}
}

func launchdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// PlistContent returns the launchd plist for a periodic sync at the given
// service binary path.
func PlistContent(self string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>run</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, Label, self, IntervalSeconds, logPath(), logPath())
}

func installLaunchd(self string) error {
	p := launchdPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(PlistContent(self)), 0644); err != nil {
		return err
	}
	exec.Command("launchctl", "unload", p).Run()
	if out, err := exec.Command("launchctl", "load", p).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %v: %s", err, out)
	}
	return nil
}

func uninstallLaunchd() error {
	p := launchdPath()
	exec.Command("launchctl", "unload", p).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func systemdDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

// ServiceContent returns the systemd unit for a periodic sync at the given
// service binary path.
func ServiceContent(self string) string {
	return fmt.Sprintf(`[Unit]
Description=Mycelium background sync

[Service]
Type=oneshot
ExecStart=%s daemon run
`, self)
}

// TimerContent returns the systemd timer that schedules the sync service.
func TimerContent() string {
	return fmt.Sprintf(`[Unit]
Description=Mycelium background sync timer

[Timer]
OnBootSec=1min
OnUnitActiveSec=%dsec
Persistent=true

[Install]
WantedBy=timers.target
`, IntervalSeconds)
}

func installSystemd(self string) error {
	dir := systemdDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "mycelium-sync.service"), []byte(ServiceContent(self)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "mycelium-sync.timer"), []byte(TimerContent()), 0644); err != nil {
		return err
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "mycelium-sync.timer").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %v: %s", err, out)
	}
	return nil
}

func uninstallSystemd() error {
	exec.Command("systemctl", "--user", "disable", "--now", "mycelium-sync.timer").Run()
	os.Remove(filepath.Join(systemdDir(), "mycelium-sync.timer"))
	os.Remove(filepath.Join(systemdDir(), "mycelium-sync.service"))
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
