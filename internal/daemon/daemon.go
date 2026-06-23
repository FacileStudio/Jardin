package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/FacileStudio/Ruche/internal/config"
)

const Label = "studio.facile.ruche-sync"
const IntervalSeconds = 300

func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p, nil
	}
	return resolved, nil
}

func logPath() string {
	return filepath.Join(config.DataDir(), "daemon.log")
}

func Run() error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	if out, err := exec.Command(self, "sync").CombinedOutput(); err != nil {
		return fmt.Errorf("sync failed: %v: %s", err, out)
	}
	cfg, err := config.LoadRucheConfig()
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
	return nil
}

func DetectAgents() []string {
	home, _ := os.UserHomeDir()
	markers := []struct {
		agent string
		path  string
	}{
		{"claude", filepath.Join(home, ".claude", "CLAUDE.md")},
		{"codex", filepath.Join(home, ".codex", "AGENTS.md")},
		{"gemini", filepath.Join(home, ".gemini", "GEMINI.md")},
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
		_, err := os.Stat(filepath.Join(systemdDir(), "ruche-sync.timer"))
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
Description=Ruche background sync

[Service]
Type=oneshot
ExecStart=%s daemon run
`, self)
}

func TimerContent() string {
	return fmt.Sprintf(`[Unit]
Description=Ruche background sync timer

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
	if err := os.WriteFile(filepath.Join(dir, "ruche-sync.service"), []byte(ServiceContent(self)), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "ruche-sync.timer"), []byte(TimerContent()), 0644); err != nil {
		return err
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "ruche-sync.timer").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %v: %s", err, out)
	}
	return nil
}

func uninstallSystemd() error {
	exec.Command("systemctl", "--user", "disable", "--now", "ruche-sync.timer").Run()
	os.Remove(filepath.Join(systemdDir(), "ruche-sync.timer"))
	os.Remove(filepath.Join(systemdDir(), "ruche-sync.service"))
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
