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

const failureUnitName = "mycelium-sync-failure.service"

// ServiceContent returns the systemd unit for a periodic sync at the given
// service binary path.
//
// OnFailure= is what stops a failed run from being silent. Under Type=oneshot
// on a timer, a nonzero exit only marks the unit failed in systemd's own state,
// and nothing reads that state until a human types journalctl by hand. That is
// how sync stayed broken for twenty hours on 2026-08-25.
func ServiceContent(self string) string {
	return fmt.Sprintf(`[Unit]
Description=Mycelium background sync
OnFailure=%s

[Service]
Type=oneshot
ExecStart=%s daemon run
`, failureUnitName, self)
}

// FailureContent returns the unit systemd starts when a sync run exits nonzero.
//
// It appends to daemon.log because that is the channel this daemon already
// reports to (runConsolidation writes there every tick) and it needs nothing
// installed beyond a shell. $$ is systemd's escape for a literal dollar sign,
// so the timestamp is taken when the failure happens rather than when the unit
// is generated.
func FailureContent() string {
	return fmt.Sprintf(`[Unit]
Description=Mycelium background sync failure notice

[Service]
Type=oneshot
ExecStart=/bin/sh -c 'echo "$$(date -Is) sync run failed, see: journalctl --user -u mycelium-sync.service -n 50" >> %s'
`, logPath())
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
	if err := os.WriteFile(filepath.Join(dir, failureUnitName), []byte(FailureContent()), 0644); err != nil {
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
	os.Remove(filepath.Join(systemdDir(), failureUnitName))
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
