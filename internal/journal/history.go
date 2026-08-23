package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Entry is one recorded operation, in the terms a human opens the history to
// ask about: when it happened, which machine did it, and what moved.
type Entry struct {
	Ref     string
	When    time.Time
	Machine string
	Message string
}

// Scope turns a path as an agent writes it into one the journal stores. Search
// reports pages relative to the memory directory, so "conventions/x.md" is what
// a human has in hand, while the history holds "memory/conventions/x.md". An
// empty path means the whole authored tree.
func Scope(path string) string {
	path = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
	if path == "" {
		return ""
	}
	for _, root := range versionedRoots() {
		if path == root || strings.HasPrefix(path, root+"/") {
			return path
		}
	}
	return "memory/" + path
}

// underScope reports whether a stored path lies inside the scope a command was
// given. An empty scope is the whole authored tree and matches everything.
func underScope(path, scoped string) bool {
	return scoped == "" || path == scoped || strings.HasPrefix(path, scoped+"/")
}

// Log returns the recorded operations that touched a path, newest first.
func Log(dataDir, path string, limit int) ([]Entry, error) {
	repo, err := git.PlainOpen(dataDir)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return nil, nil
		}
		return nil, err
	}
	head, err := repo.Head()
	if err != nil {
		return nil, nil
	}
	opts := &git.LogOptions{From: head.Hash()}
	if scoped := Scope(path); scoped != "" {
		opts.PathFilter = func(p string) bool { return underScope(p, scoped) }
	}
	iter, err := repo.Log(opts)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	return collect(iter, limit)
}

func collect(iter object.CommitIter, limit int) ([]Entry, error) {
	var entries []Entry
	err := iter.ForEach(func(c *object.Commit) error {
		entries = append(entries, entryOf(c))
		if limit > 0 && len(entries) >= limit {
			return stopWalk{}
		}
		return nil
	})
	var stop stopWalk
	if err != nil && !errors.As(err, &stop) {
		return nil, err
	}
	return entries, nil
}

// entryOf reduces a recorded operation to the four things a reader asked for:
// the ref to hand back to diff and revert, when, which machine, and what moved.
func entryOf(c *object.Commit) Entry {
	return Entry{
		Ref:     c.Hash.String()[:refLength],
		When:    c.Author.When,
		Machine: c.Author.Name,
		Message: strings.SplitN(c.Message, "\n", 2)[0],
	}
}

// refLength is how much of a hash a reader has to type back. Eight characters
// is unambiguous well past any history this will hold and still fits beside a
// date and a machine name on one line.
const refLength = 8

// stopWalk ends a history walk once enough entries are in hand. go-git treats
// any non-nil error from the callback as a stop signal and hands it straight
// back, so the sentinel has to be recognised again by the caller rather than
// reported as a failure. A type and not a package variable, because a mutable
// sentinel is exactly the shared state the standard here bans.
type stopWalk struct{}

// Error states what happened, for the one case where this escapes unrecognised.
func (stopWalk) Error() string { return "history walk stopped early" }

// Diff reports what changed in a path between a recorded state and the newest
// one, as a unified patch.
func Diff(dataDir, ref, path string) (string, error) {
	repo, err := git.PlainOpen(dataDir)
	if err != nil {
		return "", err
	}
	from, err := treeAt(repo, ref)
	if err != nil {
		return "", err
	}
	to, err := treeAt(repo, "HEAD")
	if err != nil {
		return "", err
	}
	changes, err := object.DiffTree(from, to)
	if err != nil {
		return "", err
	}
	return renderChanges(changes, Scope(path))
}

func renderChanges(changes object.Changes, scoped string) (string, error) {
	var out strings.Builder
	for _, change := range changes {
		name := change.To.Name
		if name == "" {
			name = change.From.Name
		}
		if !underScope(name, scoped) {
			continue
		}
		patch, err := change.Patch()
		if err != nil {
			return "", err
		}
		out.WriteString(neutralPatch(patch.String()))
	}
	return out.String(), nil
}

// neutralPatch drops the two header lines that name the storage rather than the
// change: one of them contains the word this surface is not allowed to print,
// and the other is a pair of object hashes no reader of a wiki page can use.
// What is left is an ordinary unified diff.
func neutralPatch(patch string) string {
	var kept []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --") || strings.HasPrefix(line, "index ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// Revert puts a path back the way it was at a recorded state: every file that
// existed then is rewritten, and anything under the path that did not exist
// then is removed.
//
// It snapshots the current state first, so the revert itself is undoable. A
// recovery command that cannot be recovered from is how a wrong ref turns one
// lost page into two.
func Revert(dataDir, ref, path string) error {
	if err := Commit(dataDir, "local: written since the last sync"); err != nil {
		return err
	}
	repo, err := git.PlainOpen(dataDir)
	if err != nil {
		return err
	}
	tree, err := treeAt(repo, ref)
	if err != nil {
		return err
	}
	scoped := Scope(path)
	restored, err := restoreFiles(dataDir, tree, scoped)
	if err != nil {
		return err
	}
	if err := removeUnknown(dataDir, scoped, restored); err != nil {
		return err
	}
	return Commit(dataDir, fmt.Sprintf("revert: %s as of %s", scopeLabel(scoped), ref))
}

func scopeLabel(scoped string) string {
	if scoped == "" {
		return "the authored tree"
	}
	return scoped
}

func treeAt(repo *git.Repository, ref string) (*object.Tree, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, fmt.Errorf("no recorded state named %q", ref)
	}
	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}

func restoreFiles(dataDir string, tree *object.Tree, scoped string) (map[string]bool, error) {
	restored := map[string]bool{}
	err := tree.Files().ForEach(func(f *object.File) error {
		if !underScope(f.Name, scoped) {
			return nil
		}
		body, err := f.Contents()
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		restored[f.Name] = true
		return os.WriteFile(target, []byte(body), 0o644)
	})
	return restored, err
}

// removeUnknown deletes what the path holds now and did not hold then. Without
// it a revert only ever adds, so undoing a sync that wrote a page leaves the
// page behind and the tree matches no state that ever existed.
func removeUnknown(dataDir, scoped string, restored map[string]bool) error {
	roots := versionedRoots()
	if scoped != "" {
		roots = []string{scoped}
	}
	for _, root := range roots {
		base := filepath.Join(dataDir, filepath.FromSlash(root))
		if err := walkRemove(dataDir, base, restored); err != nil {
			return err
		}
	}
	return nil
}

func walkRemove(dataDir, base string, restored map[string]bool) error {
	return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dataDir, path)
		if relErr != nil || restored[filepath.ToSlash(rel)] {
			return nil
		}
		return os.Remove(path)
	})
}
