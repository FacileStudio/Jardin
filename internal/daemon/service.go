package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

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
		_, err := os.Stat(filepath.Join(systemdDir(), "jardin-sync.timer"))
		return err == nil
	default:
		return false
	}
}
