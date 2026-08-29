// Package browser opens a URL in front of whoever is sitting at this machine,
// and knows when there is nobody there to open it for.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// errNoOpener is what Open reports on a machine with no way to show a page.
var errNoOpener = errors.New("no browser opener on PATH")

// Available reports whether opening a URL here can plausibly reach a human.
//
// Two things have to hold and each has failed on its own. There has to be a
// display: an SSH session on a headless box has nowhere to put a window, and
// an opener there either fails or succeeds into nothing, which is the worse
// outcome because the command then claims to have shown somebody a page nobody
// is looking at. And the opener has to exist: a container inherits DISPLAY
// from its host without inheriting xdg-open, which is how `mycelium artifact
// open` used to exit non-zero with a raw exec error instead of printing the
// link it had just recorded.
func Available() bool {
	if opener() == "" {
		return false
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return os.Getenv("SSH_CONNECTION") == ""
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// Open shows a URL or a file path in this machine's default browser. It starts
// the opener and does not wait, since the browser outlives the command that
// asked for it.
func Open(target string) error {
	path := opener()
	if path == "" {
		return errNoOpener
	}
	if runtime.GOOS == "windows" {
		return exec.Command(path, "/c", "start", "", target).Start()
	}
	return exec.Command(path, target).Start()
}

// opener is the command that puts a URL in front of a human on this OS, or the
// empty string when it is not installed.
func opener() string {
	name := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "cmd"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}
