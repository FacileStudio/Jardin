package sync

import (
	"os"
	"path/filepath"
)

const (
	manifestName = ".sync-base.json"
	conflictExt  = ".conflict"
	tokensFile   = "tokens.json"
	usagePrefix  = "usage/"
)

// Result reports what a Sync did, by category.
type Result struct {
	Uploaded      []string
	Downloaded    []string
	DeletedLocal  []string
	DeletedRemote []string
	Conflicts     []string
}

func (r *Result) Total() int {
	return len(r.Uploaded) + len(r.Downloaded) + len(r.DeletedLocal) + len(r.DeletedRemote) + len(r.Conflicts)
}

// Sync reconciles local and remote against the last-synced base manifest.
// Local-only changes are pushed, remote-only changes are pulled, deletions
// propagate both ways, and genuine conflicts pick a deterministic winner while
// preserving the loser as a sibling ".conflict" file. It never silently
// overwrites a local edit. When both sides changed and made the same change,
// the trees have already converged and the pair is just re-based.
func (c *Client) Sync(dataDir string) (*Result, error) {
	base, err := loadManifest(dataDir)
	if err != nil {
		return nil, err
	}

	localList, err := LocalTree(dataDir)
	if err != nil {
		return nil, err
	}
	remoteList, err := c.Tree()
	if err != nil {
		return nil, err
	}

	local := indexByPath(localList)
	remote := indexByPath(remoteList)

	next := make(map[string]string, len(base))
	for k, v := range base {
		next[k] = v
	}

	res := &Result{}
	for _, p := range unionPaths(local, remote, base) {
		lc := local[p].Checksum
		rc := remote[p].Checksum
		bc := base[p]

		localMod := lc != bc
		remoteMod := rc != bc

		switch {
		case !localMod && !remoteMod:
			setBase(next, p, lc)

		case localMod && !remoteMod:
			if lc == "" {
				if err := c.Delete(p); err != nil {
					return nil, err
				}
				res.DeletedRemote = append(res.DeletedRemote, p)
				delete(next, p)
			} else {
				if err := c.uploadFile(dataDir, p); err != nil {
					return nil, err
				}
				res.Uploaded = append(res.Uploaded, p)
				next[p] = lc
			}

		case !localMod && remoteMod:
			if rc == "" {
				if err := removeLocal(dataDir, p); err != nil {
					return nil, err
				}
				res.DeletedLocal = append(res.DeletedLocal, p)
				delete(next, p)
			} else {
				if err := c.downloadFile(dataDir, p); err != nil {
					return nil, err
				}
				res.Downloaded = append(res.Downloaded, p)
				next[p] = rc
			}

		default:
			if lc == rc {
				setBase(next, p, lc)
				continue
			}
			if err := c.resolveConflict(dataDir, p, local[p], remote[p], next, res); err != nil {
				return nil, err
			}
		}
	}

	if err := pruneSingleWriterConflicts(dataDir); err != nil {
		return nil, err
	}
	if err := saveManifest(dataDir, next); err != nil {
		return nil, err
	}
	return res, nil
}

// resolveConflict handles a path where both sides changed since base. Content
// always beats a deletion, so nothing is lost; a single-writer path takes the
// fresher copy outright; between two genuine edits it picks a deterministic
// winner and keeps the loser as <path>.conflict.
func (c *Client) resolveConflict(dataDir, p string, local, remote FileEntry, next map[string]string, res *Result) error {
	if local.Checksum == "" {
		if err := c.downloadFile(dataDir, p); err != nil {
			return err
		}
		res.Downloaded = append(res.Downloaded, p)
		next[p] = remote.Checksum
		res.Conflicts = append(res.Conflicts, p+" (deleted locally, edited on server — kept server copy)")
		return nil
	}
	if remote.Checksum == "" {
		if err := c.uploadFile(dataDir, p); err != nil {
			return err
		}
		res.Uploaded = append(res.Uploaded, p)
		next[p] = local.Checksum
		res.Conflicts = append(res.Conflicts, p+" (deleted on server, edited locally — kept local copy)")
		return nil
	}

	if singleWriter(p) {
		return c.resolveSingleWriter(dataDir, p, local, remote, next, res)
	}

	if localWins(local, remote) {
		remoteData, err := c.Download(p)
		if err != nil {
			return err
		}
		if err := writeConflictCopy(dataDir, p, remoteData); err != nil {
			return err
		}
		if err := c.uploadFile(dataDir, p); err != nil {
			return err
		}
		res.Uploaded = append(res.Uploaded, p)
		next[p] = local.Checksum
	} else {
		localData, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if err := writeConflictCopy(dataDir, p, localData); err != nil {
			return err
		}
		if err := c.downloadFile(dataDir, p); err != nil {
			return err
		}
		res.Downloaded = append(res.Downloaded, p)
		next[p] = remote.Checksum
	}
	res.Conflicts = append(res.Conflicts, p+" (edited on both — kept "+p+conflictExt+" backup)")
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
