// Package journal keeps a history of the authored half of the data directory,
// so a page that was deleted or overwritten can be recovered. On 2026-08-19 a
// reconcile removed 246 pages and nothing could bring them back; the guard in
// internal/sync stops a plan that large, and this is what makes a smaller one
// survivable.
//
// Nothing here reaches an agent. The journal has no command of its own, adds
// no word to any help text, and is read only through the history commands a
// human runs. Agents know sync and never know storage: see
// conventions/mycelium-agent-surface.md.
//
// go-git rather than a shelled-out binary, because the server image is
// distroless with no shell and the client installs as one static binary with no
// runtime dependencies. Requiring git on PATH for core memory regresses both.
package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// versionedRoots is the authored content the journal keeps. Everything else in
// the data directory is telemetry or local state: events, usage, claims, runs
// and sessions all churn on a timer and would bury a page's history in noise.
//
// extensions holds the typed model code that flows call. SPEC.md lists four
// roots and not this one, because the directory did not exist when it was
// written. It is authored, it syncs, and a bad reconcile loses it exactly the
// way it loses a page, so it is protected the same way.
//
// A function rather than a package variable so no caller can append to the set
// that decides what is protected.
func versionedRoots() []string {
	return []string{"memory", "rules", "skills", "flows", "extensions"}
}

// Health is what a health check needs to know: whether anything has been
// recorded on this machine, and what the last thing was.
type Health struct {
	Started bool
	Last    Entry
}

// Inspect reports the state of the journal without changing it.
//
// It exists because the journal introduced a failure that persists until a
// human acts. Every commit failure is a warning on a sync that still succeeds,
// which is the right trade, and it also means a machine can stop recording and
// keep looking fine. The same hole was found in the "last sync" check in
// v0.20.0: a check that cannot go red is not a check.
//
// A missing repository and one that cannot be read are different answers. The
// first is a machine that has not run init since the journal shipped, and the
// second is damage.
func Inspect(dataDir string) (Health, error) {
	repo, err := git.PlainOpen(dataDir)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return Health{}, nil
		}
		return Health{}, fmt.Errorf("the history cannot be opened: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return Health{}, nil
		}
		return Health{}, fmt.Errorf("the history cannot be read: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return Health{}, fmt.Errorf("the newest entry is unreadable: %w", err)
	}
	return Health{Started: true, Last: entryOf(commit)}, nil
}

// Init prepares the journal and records whatever the data directory already
// holds as its first commit, so an install that predates versioning starts with
// a full snapshot rather than an empty history.
//
// Idempotent: a second run finds nothing uncommitted and writes nothing.
func Init(dataDir string) error {
	return Commit(dataDir, "init: snapshot the authored tree")
}

// Commit records one operation. It stages the versioned roots by name and
// nothing else, so a commit cannot sweep in a telemetry directory even if the
// ignore file has been deleted, and it writes no commit at all when nothing
// changed.
//
// One commit per operation, never per file: a sync that pulls six pages is one
// commit.
func Commit(dataDir, message string) error {
	unlock, err := lockJournal(dataDir)
	if err != nil {
		return err
	}
	defer unlock()

	wt, err := worktree(dataDir)
	if err != nil {
		return err
	}
	if err := stageRoots(wt, dataDir); err != nil {
		return err
	}
	staged, err := stagedCount(wt)
	if err != nil || staged == 0 {
		return err
	}
	_, err = wt.Commit(message, &git.CommitOptions{Author: signature()})
	return err
}

// worktree opens the journal's repository, creating it on first use. Commit
// bootstraps through here rather than relying on Init, because every machine
// that installed before the journal existed has already run init once and will
// never run it again.
func worktree(dataDir string) (*git.Worktree, error) {
	repo, err := openRepo(dataDir)
	if err != nil {
		return nil, err
	}
	return repo.Worktree()
}

func openRepo(dataDir string) (*git.Repository, error) {
	repo, err := git.PlainOpen(dataDir)
	if err == nil {
		return repo, ensureIgnore(dataDir)
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := ensureIgnore(dataDir); err != nil {
		return nil, err
	}
	return git.PlainInit(dataDir, false)
}

// stageRoots stages the versioned directories that exist. A missing root is
// skipped rather than reported: flows and skills are absent on a fresh machine
// until something writes one, and a first sync must not fail because of it.
func stageRoots(wt *git.Worktree, dataDir string) error {
	for _, root := range versionedRoots() {
		if _, err := os.Stat(filepath.Join(dataDir, root)); err != nil {
			continue
		}
		if err := wt.AddWithOptions(&git.AddOptions{Path: root}); err != nil {
			return fmt.Errorf("staging %s: %w", root, err)
		}
	}
	return nil
}

// stagedCount reports how many paths the index holds that HEAD does not agree
// with. Untracked entries are not counted: the ignore file keeps telemetry out
// of the status, and anything ignored that slipped past it must not be able to
// make an otherwise empty commit look like a real one.
func stagedCount(wt *git.Worktree) (int, error) {
	status, err := wt.Status()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range status {
		if s.Staging != git.Unmodified && s.Staging != git.Untracked {
			n++
		}
	}
	return n, nil
}

// signature attributes a commit to the machine that made it, which is the
// question a human opens the history to answer: not what changed, but where it
// came from. The address is synthetic because there is no mailbox behind it.
func signature() *object.Signature {
	machine := config.MachineName()
	return &object.Signature{
		Name:  machine,
		Email: "mycelium@" + machine,
		When:  time.Now(),
	}
}

// ensureIgnore writes the exclusion list, rewriting it only when it differs so
// a sync does not touch the file's timestamp on every run.
//
// It is an ordinary denylist and not the exclude-everything-then-allow-four
// form, because that shape depends on negation ordering and go-git's ignore
// matcher is a reimplementation rather than git's own. The staging set is the
// real guarantee; this file only decides what a human sees in a status.
func ensureIgnore(dataDir string) error {
	path := filepath.Join(dataDir, ".gitignore")
	want := ignoreBody()
	if have, err := os.ReadFile(path); err == nil && string(have) == want {
		return nil
	}
	return os.WriteFile(path, []byte(want), 0o644)
}

func ignoreBody() string {
	return strings.Join([]string{
		"# Written by mycelium. Only the authored tree is versioned: memory,",
		"# rules, skills and flows. Everything below churns on a timer and",
		"# would bury a page's history in noise.",
		"/.*",
		"/claims/",
		"/events/",
		"/machines/",
		"/runs/",
		"/sessions/",
		"/usage/",
		"/tokens.json",
		"*.conflict",
		"*.log",
		"node_modules/",
		"",
	}, "\n")
}
