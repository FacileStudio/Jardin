package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/server"
	hsync "github.com/FacileStudio/Mycelium/internal/sync"
)

// captureStderr collects what the ui helpers wrote while run executed. They
// print to os.Stderr rather than returning a string, and the wording of a
// refusal is the whole subject here, so reading the stream is the only way to
// assert on it.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = w
	run()
	os.Stderr = original
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	return string(out)
}

// syncHarness points the CLI at a throwaway server and a throwaway data
// directory. Nothing here may reach ~/.mycelium: a reconcile against an empty
// server emptied it on 2026-08-25 and it came back one commit at a time.
func syncHarness(t *testing.T) (string, string) {
	t.Helper()
	serverDir := t.TempDir()
	clientDir := t.TempDir()
	ts := httptest.NewServer(server.New(serverDir, "").Handler())
	t.Cleanup(ts.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DATA_DIR", clientDir)
	t.Setenv(config.URLEnv, ts.URL)
	t.Setenv(config.TokenEnv, "")
	return clientDir, serverDir
}

// seedBoth puts n identical pages on both sides and syncs once, so the base
// records them and a later removal reads as a deletion rather than as a file
// that never existed.
func seedBoth(t *testing.T, clientDir, serverDir string, n int) []string {
	t.Helper()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rel := fmt.Sprintf("memory/page%02d.md", i)
		writePage(t, clientDir, rel)
		writePage(t, serverDir, rel)
		paths = append(paths, rel)
	}
	if err := syncCmd.RunE(syncCmd, nil); err != nil {
		t.Fatalf("seeding sync failed: %v", err)
	}
	return paths
}

func writePage(t *testing.T, dir, rel string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removePages(t *testing.T, dir string, paths []string) {
	t.Helper()
	for _, p := range paths {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
}

// setForce puts the flag back on the way out. Cobra flags live on a package
// level command, so a test that leaves --force on hands it to the next one.
func setForce(t *testing.T, on bool) {
	t.Helper()
	if err := syncCmd.Flags().Set("force", fmt.Sprint(on)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syncCmd.Flags().Set("force", "false") })
}

// requireNoTerminal skips a test that can only mean anything off a terminal.
// `go test` pipes the binary's stdout, so this holds in every normal run; it
// would not if somebody ran the compiled test binary straight in a shell.
func requireNoTerminal(t *testing.T) {
	t.Helper()
	if interactiveTerminal() {
		t.Skip("this test asserts what happens with no terminal attached")
	}
}

func TestForceIsRefusedWhenNoTerminalIsAttached(t *testing.T) {
	var err error
	out := captureStderr(t, func() {
		err = requireTerminal(syncCmd, false, forceNeedsTerminal)
	})
	if !errors.Is(err, errNotInteractive) {
		t.Fatalf("--force off a terminal must refuse, got %v", err)
	}
	if !strings.Contains(out, "terminal") {
		t.Fatalf("the refusal must say a terminal is what is missing, got %q", out)
	}
	if !strings.Contains(out, "Nothing was changed") {
		t.Fatalf("the refusal must say nothing was destroyed, got %q", out)
	}
}

func TestForceIsAcceptedFromATerminal(t *testing.T) {
	var err error
	out := captureStderr(t, func() {
		err = requireTerminal(syncCmd, true, forceNeedsTerminal)
	})
	if err != nil {
		t.Fatalf("a human at a prompt is who --force is for, got %v", err)
	}
	if out != "" {
		t.Fatalf("an accepted --force says nothing, got %q", out)
	}
}

// The 2026-08-25 shape end to end: eleven pages gone from the server, --force
// typed by something with no terminal. The files have to still be there
// afterwards, because a guard that refuses after deleting is not a guard.
func TestForceRefusesBeforeItDeletesAnything(t *testing.T) {
	requireNoTerminal(t)
	clientDir, serverDir := syncHarness(t)
	paths := seedBoth(t, clientDir, serverDir, 11)
	removePages(t, serverDir, paths)
	setForce(t, true)

	var err error
	captureStderr(t, func() { err = syncCmd.RunE(syncCmd, nil) })
	if !errors.Is(err, errNotInteractive) {
		t.Fatalf("expected the terminal refusal, got %v", err)
	}
	for _, p := range paths {
		if _, statErr := os.Stat(filepath.Join(clientDir, filepath.FromSlash(p))); statErr != nil {
			t.Fatalf("%s was deleted by a refused --force", p)
		}
	}
}

// The daemon's exact call: `mycelium sync`, no flag, stdout piped, every 60
// seconds. A guard that stops this stops the fleet syncing at all.
func TestSyncUnderTheLimitStillRunsUnattended(t *testing.T) {
	requireNoTerminal(t)
	clientDir, serverDir := syncHarness(t)
	paths := seedBoth(t, clientDir, serverDir, 3)
	removePages(t, serverDir, paths)

	if err := syncCmd.RunE(syncCmd, nil); err != nil {
		t.Fatalf("three deletions with no --force is ordinary work, got %v", err)
	}
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(clientDir, filepath.FromSlash(p))); err == nil {
			t.Fatalf("%s survived a sync that was under the limit", p)
		}
	}
}

func TestPullIsRefusedWhenNoTerminalIsAttached(t *testing.T) {
	requireNoTerminal(t)
	clientDir, serverDir := syncHarness(t)
	writePage(t, serverDir, "memory/only-on-the-server.md")

	var err error
	captureStderr(t, func() { err = pullCmd.RunE(pullCmd, nil) })
	if !errors.Is(err, errNotInteractive) {
		t.Fatalf("pull overwrites local files, so it must refuse off a terminal, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(clientDir, "memory", "only-on-the-server.md")); statErr == nil {
		t.Fatal("a refused pull wrote to the data directory anyway")
	}
}

// Push is the other direction and is deliberately not gated. It destroys
// nothing locally, and running it from a script is how the wiki got back onto
// the server after the 2026-08-25 wipe.
func TestPushIsNotGatedByTheTerminal(t *testing.T) {
	requireNoTerminal(t)
	clientDir, serverDir := syncHarness(t)
	writePage(t, clientDir, "memory/written-here.md")

	if err := pushCmd.RunE(pushCmd, nil); err != nil {
		t.Fatalf("push must run unattended, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(serverDir, "memory", "written-here.md")); err != nil {
		t.Fatalf("push did not reach the server: %v", err)
	}
}

// The refusal an agent reads must not contain the command that defeats it. This
// is the whole fix: the message, not the limit, is what failed twice.
func TestBulkDeleteRefusalNamesTheFlagOnlyForAHuman(t *testing.T) {
	refusal := &hsync.BulkDeleteError{
		Local:       []string{"memory/a.md", "memory/b.md"},
		LocalFiles:  111,
		RemoteFiles: 20,
	}

	human := captureStderr(t, func() { reportBulkDelete(syncCmd, true, refusal) })
	if !strings.Contains(human, "mycelium sync --force") {
		t.Fatalf("a human at a prompt is who the flag is for, got %q", human)
	}

	agent := captureStderr(t, func() { reportBulkDelete(syncCmd, false, refusal) })
	if strings.Contains(agent, "force") {
		t.Fatalf("the non-interactive refusal must not name its own bypass, got %q", agent)
	}
	if !strings.Contains(agent, "terminal") {
		t.Fatalf("it must say a human has to review this on a terminal, got %q", agent)
	}
	if !strings.Contains(agent, "api/sync/tree") {
		t.Fatalf("it must name the check that would have caught an empty server, got %q", agent)
	}
	if !strings.Contains(agent, "2 here, 0 on the server") {
		t.Fatalf("it must name how many deletions and in which direction, got %q", agent)
	}
}

// Both wipes were a server that came back empty, and that is one subtraction
// away from the numbers the reconcile already had. A refusal that does not
// print them makes the reader go and fetch the tree by hand.
func TestBulkDeleteRefusalReportsWhatEachSideHolds(t *testing.T) {
	refusal := &hsync.BulkDeleteError{
		Local:       []string{"memory/a.md"},
		LocalFiles:  111,
		RemoteFiles: 20,
	}
	out := captureStderr(t, func() { reportBulkDelete(syncCmd, true, refusal) })
	if !strings.Contains(out, "The server holds 20 files and this machine holds 111") {
		t.Fatalf("the refusal must name both file counts, got %q", out)
	}
}
