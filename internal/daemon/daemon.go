package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/usage"
)

const Label = "studio.facile.jardin-sync"

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
	cfg, err := config.LoadJardinConfig()
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
	cfg, err := config.LoadJardinConfig()
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
	}
	var found []string
	for _, m := range markers {
		if _, err := os.Stat(m.path); err == nil {
			found = append(found, m.agent)
		}
	}
	return found
}

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

func Installed() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := os.Stat(launchdPath())
		return err == nil
	case "linux":
		_, err := os.Stat(filepath.Join(systemdDir(), "jardin-sync.timer"))
		return err == nil
	default:
		return false
	}
}

func launchdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

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

func ServiceContent(self string) string {
	return fmt.Sprintf(`[Unit]
Description=Jardin background sync

[Service]
Type=oneshot
ExecStart=%s daemon run
`, self)
}

func TimerContent() string {
	return fmt.Sprintf(`[Unit]
Description=Jardin background sync timer

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
	if err := os.WriteFile(filepath.Join(dir, "jardin-sync.service"), []byte(ServiceContent(self)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "jardin-sync.timer"), []byte(TimerContent()), 0644); err != nil {
		return err
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "jardin-sync.timer").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %v: %s", err, out)
	}
	return nil
}

func uninstallSystemd() error {
	exec.Command("systemctl", "--user", "disable", "--now", "jardin-sync.timer").Run()
	os.Remove(filepath.Join(systemdDir(), "jardin-sync.timer"))
	os.Remove(filepath.Join(systemdDir(), "jardin-sync.service"))
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
