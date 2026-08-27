package artifacts

import (
	"os"
	"os/exec"
	"runtime"
)

// HasDisplay reports whether this machine can put a browser in front of a
// human.
//
// An agent working over SSH on a headless box has nowhere to open a page, and
// xdg-open there either fails or succeeds into nothing. The second is the worse
// outcome: the command claims to have shown somebody something it did not, and
// the page is left waiting on a machine nobody is looking at.
func HasDisplay() bool {
	if runtime.GOOS == "darwin" {
		return os.Getenv("SSH_CONNECTION") == ""
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// Open shows an artifact in this machine's default browser.
func Open(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, path).Start()
}
