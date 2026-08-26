package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// pageMode is what a wiki page keeps. os.CreateTemp makes a file 0600, so a
	// staged file is chmodded before the rename rather than tightening the corpus.
	pageMode = 0644

	// stagedSuffix keeps a file that has been written but not yet renamed out of
	// sync. internal/sync's syncSkip excludes this exact suffix and no other
	// temporary shape, so a staged page named anything else is a whole second
	// copy of a wiki page published to the server and to every other machine the
	// moment a crash leaves one behind. internal/consolidate's writer stages
	// under the same suffix for the same reason.
	stagedSuffix = ".tmp"
)

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
// one transaction, because POSIX has no multi-file rename, so a crash between
// them can still land a finding without its log line: a window three syscalls
// wide, while the failures that actually happen — a full disk, a read-only
// wiki, a bad page — all happen during staging, where nothing has been renamed
// yet.
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

// stage writes one file's next content beside it, ready to be renamed into
// place and named so that sync will not carry it if the rename never happens.
func stage(f edit) (s edit, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".*"+stagedSuffix)
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
