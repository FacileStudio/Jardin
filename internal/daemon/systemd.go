package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func systemdDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

// ServiceContent returns the systemd unit for a periodic sync at the given
// service binary path.
func ServiceContent(self string) string {
	return fmt.Sprintf(`[Unit]
Description=Jardin background sync

[Service]
Type=oneshot
ExecStart=%s daemon run
`, self)
}

// TimerContent returns the systemd timer that schedules the sync service.
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
