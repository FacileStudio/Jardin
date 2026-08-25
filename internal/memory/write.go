package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	indexFile = "index.md"
	logFile   = "log.md"

	// logOperation is the verb a filed finding carries in log.md, and always this
	// one: the convention's other four describe edits to pages that exist.
	logOperation = "ingest"

	// pageMode is what a wiki page keeps. os.CreateTemp makes a file 0600, so a
	// staged file is chmodded before the rename rather than tightening the corpus.
	pageMode = 0644
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
func Add(dataDir string, f Finding, now time.Time, push Push) (Result, error) {
	if err := f.validate(); err != nil {
		return Result{}, err
	}
	day := now.Format(dayLayout)
	block := fmt.Sprintf("### %s\n**Date**: %s\n**Source**: %s\n%s\n",
		strings.TrimSpace(f.Title), day, strings.TrimSpace(f.Source), strings.TrimSpace(f.Body))
	if err := refuseFrench(block + "\n" + f.Log); err != nil {
		return Result{}, err
	}
	path, key, err := pagePath(dataDir, f.Page)
	if err != nil {
		return Result{}, err
	}
	page, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("cannot file a finding in %s: %w", key, err)
	}
	stamped, err := bumpUpdated(string(page), day)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", key, err)
	}
	index := filepath.Join(memoryRoot(dataDir), indexFile)
	history := filepath.Join(memoryRoot(dataDir), logFile)
	nextIndex, pointed := indexPointer(readOrEmpty(index), key, f.Title)
	err = writeAll([]edit{
		{path: path, data: strings.TrimRight(stamped, "\n") + "\n\n" + block},
		{path: index, data: nextIndex},
		{path: history, data: appendLog(readOrEmpty(history), day, f.Log)},
	})
	if err != nil {
		return Result{}, err
	}
	res := Result{Page: key, Indexed: pointed}
	if push != nil {
		res.SyncErr = push()
	}
	return res, nil
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

// bumpUpdated rewrites the page's `updated:` stamp, adding the field when the
// header lacks it. A page with no frontmatter is refused rather than repaired:
// the header carries what retrieval and ratification read, and inventing one
// files a finding on a page the wiki only half sees.
func bumpUpdated(content, day string) (string, error) {
	const fence = "---\n"
	if !strings.HasPrefix(content, fence) {
		return "", fmt.Errorf("the page has no YAML frontmatter, so there is no `updated:` to stamp")
	}
	end := strings.Index(content[len(fence):], "\n---")
	if end < 0 {
		return "", fmt.Errorf("the page's YAML frontmatter is never closed")
	}
	head := strings.Split(content[len(fence):len(fence)+end], "\n")
	rest := content[len(fence)+end:]
	for i, line := range head {
		if strings.HasPrefix(line, "updated:") {
			head[i] = "updated: " + day
			return fence + strings.Join(head, "\n") + rest, nil
		}
	}
	return fence + strings.Join(append(head, "updated: "+day), "\n") + rest, nil
}

// indexPointer returns index.md with a pointer to the page, and whether it
// added one. A page the index already names keeps the line it has: the index is
// a router with one line per page, and a line per finding is the rot itself.
// The section is the page's own directory, capitalised, because the wiki's
// directories and its index headings are the same words.
func indexPointer(index, page, title string) (string, bool) {
	if strings.Contains(index, "("+page+")") {
		return index, false
	}
	name := strings.TrimSuffix(filepath.Base(page), ".md")
	line := fmt.Sprintf("- [%s](%s): %s", name, page, strings.TrimSpace(title))
	section := "Pages"
	if dir, _, found := strings.Cut(page, "/"); found && dir != "" {
		section = strings.ToUpper(dir[:1]) + dir[1:]
	}
	return insertInSection(index, section, line), true
}

// insertInSection puts the line at the end of its section's entries, starting
// that section at the end of the file when the index has none. Trailing blank
// lines stay below the insertion point, so the spacing between sections lives.
func insertInSection(index, section, line string) string {
	head := "## " + section
	if strings.TrimSpace(index) == "" {
		return strings.Join([]string{head, "", line}, "\n") + "\n"
	}
	lines := strings.Split(strings.TrimRight(index, "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == head {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return strings.Join(append(lines, "", head, "", line), "\n") + "\n"
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := append(append([]string{}, lines[:end]...), line)
	return strings.Join(append(out, lines[end:]...), "\n") + "\n"
}

// appendLog adds the history line. log.md is append-only by invariant, so this
// only grows the file.
func appendLog(content, day, description string) string {
	line := fmt.Sprintf("## [%s] %s | %s\n", day, logOperation, strings.TrimSpace(description))
	if strings.TrimSpace(content) == "" {
		return line
	}
	return strings.TrimRight(content, "\n") + "\n" + line
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

// edit is one file's next content in full, and once staged the temp file waiting
// to take its place. Everything is composed first, so a bad page or French prose
// fails while the wiki is still untouched.
type edit struct {
	path string
	data string
	tmp  string
}

// writeAll makes the edits land together. Every file is written whole and
// synced beside its target before anything moves, so the only work left is a
// rename, and a rename is atomic: no reader sees half a page, and nothing moves
// at all if any file cannot be staged. The commit is three renames rather than
// one transaction, because POSIX has no
// multi-file rename, so a crash between them can still land a finding without
// its log line: a window three syscalls wide, while the failures that actually
// happen — a full disk, a read-only wiki, a bad page — all happen during
// staging, where nothing has been renamed yet.
func writeAll(files []edit) error {
	var ready []edit
	defer func() {
		for _, e := range ready {
			os.Remove(e.tmp)
		}
	}()
	for _, f := range files {
		staged, err := stage(f)
		if err != nil {
			return fmt.Errorf("failed to stage %s: %w", f.path, err)
		}
		ready = append(ready, staged)
	}
	for _, e := range ready {
		if err := os.Rename(e.tmp, e.path); err != nil {
			return fmt.Errorf("failed to write %s: %w", e.path, err)
		}
	}
	return nil
}

// stage writes one file's next content beside it, ready to be renamed into place.
func stage(f edit) (s edit, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".*")
	if err != nil {
		return edit{}, err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.WriteString(f.data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), modeOf(f.path))
	}

	if err != nil {
		return edit{}, err
	}
	f.tmp = tmp.Name()
	return f, nil
}

// modeOf reports the permissions a staged file needs to replace its target
// without changing them.
func modeOf(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return pageMode
}
