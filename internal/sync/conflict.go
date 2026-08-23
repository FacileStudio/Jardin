package sync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolveConflict handles a path where both sides changed since base. Content
// always beats a deletion, so nothing is lost; a single-writer path takes the
// fresher copy outright; between two genuine edits it picks a deterministic
// winner and keeps the loser under .conflicts/.
func (c *Client) resolveConflict(r reconcile) error {
	if r.local.Checksum == "" {
		return c.keepServerCopy(r)
	}
	if r.remote.Checksum == "" {
		return c.keepLocalCopy(r)
	}
	if singleWriter(r.path) {
		return c.resolveSingleWriter(r)
	}

	if localWins(r.local, r.remote) {
		if err := c.backupRemoteAndPushLocal(r); err != nil {
			return err
		}
	} else if err := c.backupLocalAndPullRemote(r); err != nil {
		return err
	}
	r.res.Conflicts = append(r.res.Conflicts, r.path+" (edited on both, losing copy at "+conflictDir+"/"+r.path+")")
	return nil
}

// keepServerCopy restores a file deleted here but edited on the server. The
// edit exists nowhere else, while the deletion can be repeated for free, so
// there is only one version to keep and no backup is written.
func (c *Client) keepServerCopy(r reconcile) error {
	if err := c.downloadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Downloaded = append(r.res.Downloaded, r.path)
	r.next[r.path] = r.remote.Checksum
	r.res.Conflicts = append(r.res.Conflicts, r.path+" (deleted locally, edited on server — kept server copy)")
	return nil
}

// keepLocalCopy is the mirror: the server dropped a file this machine edited,
// so the local copy is pushed back up.
func (c *Client) keepLocalCopy(r reconcile) error {
	if err := c.uploadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Uploaded = append(r.res.Uploaded, r.path)
	r.next[r.path] = r.local.Checksum
	r.res.Conflicts = append(r.res.Conflicts, r.path+" (deleted on server, edited locally — kept local copy)")
	return nil
}

// backupRemoteAndPushLocal settles an edit-vs-edit the local side won: the
// server's version is fetched and parked under .conflicts/ before the local
// file is pushed over it, so the losing edit survives on disk.
func (c *Client) backupRemoteAndPushLocal(r reconcile) error {
	remoteData, err := c.Download(r.path)
	if err != nil {
		return err
	}
	if err := writeConflictCopy(r.dataDir, r.path, remoteData); err != nil {
		return err
	}
	if err := c.uploadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Uploaded = append(r.res.Uploaded, r.path)
	r.next[r.path] = r.local.Checksum
	return nil
}

// backupLocalAndPullRemote settles an edit-vs-edit the server won: the local
// file is copied under .conflicts/ before being overwritten by the download.
func (c *Client) backupLocalAndPullRemote(r reconcile) error {
	localData, err := os.ReadFile(filepath.Join(r.dataDir, filepath.FromSlash(r.path)))
	if err != nil {
		return err
	}
	if err := writeConflictCopy(r.dataDir, r.path, localData); err != nil {
		return err
	}
	if err := c.downloadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Downloaded = append(r.res.Downloaded, r.path)
	r.next[r.path] = r.remote.Checksum
	return nil
}

// localWins picks the conflict winner: newer modification time, falling back to
// the larger checksum so the choice is identical on every machine (convergent,
// never ping-pongs).
func localWins(local, remote FileEntry) bool {
	lt, lok := parseTime(local.ModTime)
	rt, rok := parseTime(remote.ModTime)
	if lok && rok && !lt.Equal(rt) {
		return lt.After(rt)
	}
	return local.Checksum >= remote.Checksum
}

// conflictPath is where the losing copy of an edit-vs-edit lands. It mirrors
// the page's own path under a dot-directory instead of sitting beside it: a
// file called foo.md.conflict inside memory/ is storage showing through into
// what an agent reads, and the dot prefix already keeps the whole tree out of
// sync. The copy keeps its real extension, so it still opens as what it is.
func conflictPath(dataDir, p string) string {
	return filepath.Join(dataDir, conflictDir, filepath.FromSlash(p))
}

// writeConflictCopy parks one version of a page that lost an edit-vs-edit.
// Conflict markers are never written into the page itself: a page always holds
// one readable version, which is the complaint every git-backed notes tool
// collects.
func writeConflictCopy(dataDir, p string, data []byte) error {
	full := conflictPath(dataDir, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// backedUpPath maps a file in the data directory to the page it is a backup of,
// or "" when it is not a backup. Two layouts answer to this: the current one
// mirrors the page under .conflicts/, and the one before it parked a sibling
// <path>.conflict, which is still on disk on any machine that wrote one before
// the move and which nothing else will ever clear.
func backedUpPath(rel string) string {
	if page, ok := strings.CutPrefix(rel, conflictDir+"/"); ok {
		return page
	}
	if page, ok := strings.CutSuffix(rel, conflictExt); ok {
		return page
	}
	return ""
}

// ConflictBackups lists the pages holding an unresolved conflict copy, sorted,
// in either layout. doctor is the caller: a conflict is an event a human
// resolves, and where the copies live is this package's business, not its.
func ConflictBackups(dataDir string) []string {
	var pages []string
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(dataDir, path)
		if relErr != nil {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if info.IsDir() {
			if !strings.HasPrefix(slash, conflictDir) && skipWalkDir(slash) {
				return filepath.SkipDir
			}
			return nil
		}
		if page := backedUpPath(slash); page != "" {
			pages = append(pages, page)
		}
		return nil
	}
	if err := filepath.Walk(dataDir, walk); err != nil {
		return nil
	}
	sort.Strings(pages)
	return pages
}
