package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func pathWithOpener(t *testing.T, install bool) {
	t.Helper()
	dir := t.TempDir()
	if install {
		name := "xdg-open"
		if runtime.GOOS == "darwin" {
			name = "open"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("install opener: %v", err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestAvailableIsFalseWithoutAnOpener(t *testing.T) {
	pathWithOpener(t, false)
	t.Setenv("DISPLAY", ":0")
	t.Setenv("SSH_CONNECTION", "")

	if Available() {
		t.Fatal("a machine with a display but no opener cannot show a page")
	}
	if err := Open("https://example.invalid"); err != errNoOpener {
		t.Fatalf("open without an opener: got %v, want %v", err, errNoOpener)
	}
}

func TestAvailableNeedsADisplayToo(t *testing.T) {
	pathWithOpener(t, true)
	t.Setenv("SSH_CONNECTION", "")

	if runtime.GOOS == "darwin" {
		if !Available() {
			t.Fatal("a Mac nobody is ssh'd into can show a page")
		}
		t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
		if Available() {
			t.Fatal("an ssh session into a Mac has no window to open")
		}
		return
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if Available() {
		t.Fatal("a headless box has nowhere to put a window")
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if !Available() {
		t.Fatal("wayland counts as a display")
	}
}
