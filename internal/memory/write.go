package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/lockfile"
)

const (
	indexFile = "index.md"
	logFile   = "log.md"

	// logOperation is the verb a filed finding carries in log.md, and always this
	// one: the convention's other four describe edits to pages that exist.
	logOperation = "ingest"

	// wikiLock is the file every writer of index.md and log.md takes before its
	// read, and wikiLockWait bounds how long one waits for another to finish.
	// The critical section is three reads and three renames, so a two-second
	// ceiling is already orders of magnitude more headroom than it needs.
	//
	// A dotfile directly under the data directory, never under memory/: sync
	// excludes a path starting with a dot and nothing else here, so a lock
	// anywhere else would travel to the server as an ordinary wiki file.
	wikiLock     = ".memory.lock"
	wikiLockWait = 2 * time.Second
)

// Finding is one entry as the wiki convention writes it. Log is a field rather
// than something derived from the change, because the convention asks a log line
// for what was wrong before and what still holds, and a diff carries neither.
type Finding struct {
	Page   string
	Title  string
	Source string
	Body   string
	Log    string
}

// Result reports what one Add did. SyncErr is the outcome of the follow-up sync
// and never a failure of the write: by the time it is set, every byte is on disk.
type Result struct {
	Page    string
	Indexed bool
	SyncErr error
}

// Push sends the wiki on to the other machines, injected so this package keeps
// no opinion about how a machine reaches its server.
type Push func() error

// Add files one finding and does every piece of bookkeeping the convention asks
// for in one call: the finding on its page, the page's `updated:` stamp, the
// index pointer, and the log line, in the shape the corpus already uses so that
// search's date parser reads them. Either all four land or none do. Done by
// hand the step that gets skipped is always the index, because it is the one
// edit that is not where the writing happened, and that is how an index rots.
//
// The prose is composed and checked before anything is locked or opened, so a
// finding that is half filled in or written in French fails while the wiki is
// still untouched and while no other writer is being made to wait.
func Add(dataDir string, f Finding, now time.Time, push Push) (Result, error) {
	if err := f.validate(); err != nil {
		return Result{}, err
	}
	day := now.Format(dayLayout)
	block := findingBlock(f, day)
	if err := refuseFrench(block + "\n" + f.Log); err != nil {
		return Result{}, err
	}
	res, err := fileFinding(dataDir, f, day, block)
	if err != nil {
		return Result{}, err
	}
	if push != nil {
		res.SyncErr = push()
	}
	return res, nil
}

// fileFinding makes the four edits, holding the wiki lock across the read and
// the write so two agents filing at the same moment cannot each compose a
// version of index.md and log.md from the same starting point and then take
// turns overwriting the other. An agent files a finding when its task ends,
// which is exactly when several agents on one machine finish at once, and
// log.md is append-only by invariant, so a line lost that way is lost for good.
//
// Sync deliberately does not take this lock. Serialising a reconcile against a
// write would put a network round trip between an agent and its own finding,
// and a wiki that stalls a write on a slow server is the thing being
// local-first exists to prevent. What is left is a sync landing inside the few
// milliseconds below, which the journal commits on both sides of and
// `mycelium memory revert` can undo.
func fileFinding(dataDir string, f Finding, day, block string) (Result, error) {
	path, key, err := pagePath(dataDir, f.Page)
	if err != nil {
		return Result{}, err
	}
	release, err := lockWiki(dataDir)
	if err != nil {
		return Result{}, err
	}
	defer release()

	page, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("cannot file a finding in %s: %w", key, err)
	}
	stamped, err := bumpUpdated(string(page), day)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", key, err)
	}
	front, _ := frontmatter(string(page))
	index := filepath.Join(memoryRoot(dataDir), indexFile)
	history := filepath.Join(memoryRoot(dataDir), logFile)
	nextIndex, pointed := indexPointer(readOrEmpty(index), key, front.title, f.Title)
	err = writeAll([]edit{
		{path: path, data: strings.TrimRight(stamped, "\n") + "\n\n" + block},
		{path: index, data: nextIndex},
		{path: history, data: appendLog(readOrEmpty(history), day, f.Log)},
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Page: key, Indexed: pointed}, nil
}

// lockWiki serialises the writers of index.md and log.md, naming the work in
// the words a reader of the refusal needs rather than the lock's own.
func lockWiki(dataDir string) (func(), error) {
	release, err := lockfile.Take(dataDir, wikiLock, wikiLockWait)
	if errors.Is(err, lockfile.ErrHeld) {
		return nil, fmt.Errorf("another mycelium process is writing to the wiki")
	}
	return release, err
}

// findingBlock renders one finding in the shape the corpus already uses, which
// is what makes search's date parser read it.
func findingBlock(f Finding, day string) string {
	return fmt.Sprintf("### %s\n**Date**: %s\n**Source**: %s\n%s\n",
		strings.TrimSpace(f.Title), day, strings.TrimSpace(f.Source), strings.TrimSpace(f.Body))
}

// validate refuses a half-filled finding: one with no source cannot be checked
// later, one with no log line leaves the history saying nothing happened.
func (f Finding) validate() error {
	for _, field := range [][2]string{{f.Page, "a page"}, {f.Title, "--title"},
		{f.Source, "--source"}, {f.Body, "--body or --body-stdin"}, {f.Log, "--log"}} {
		if strings.TrimSpace(field[0]) == "" {
			return fmt.Errorf("a finding needs %s", field[1])
		}
	}
	return nil
}

// refuseFrench blocks a write carrying French prose. The sync-side check in
// language.go only warns, because failing there would lock a machine out of
// pulling the very fix it needs; this is the blocking gate that comment points
// at, where the writer is still here and the text still in hand.
func refuseFrench(text string) error {
	found := FrenchLines(text)
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("the wiki is English-only and this reads as French: %s",
		strings.Split(text, "\n")[found[0]-1])
}

// pagePath resolves "tools/mycelium" as readily as "tools/mycelium.md" to an
// absolute path and a wiki-relative key, refusing anything outside the wiki.
func pagePath(dataDir, page string) (string, string, error) {
	key, err := pageKey(page)
	if err != nil {
		return "", "", err
	}
	if !strings.HasSuffix(key, ".md") {
		key += ".md"
	}
	return filepath.Join(memoryRoot(dataDir), filepath.FromSlash(key)), key, nil
}

// readOrEmpty returns a file's content, or nothing when it is not there: a wiki
// missing index.md or log.md gets one rather than an error.
func readOrEmpty(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}
