package sync

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

// reconcile carries everything settling one path needs: where the tree lives,
// which path is being decided, what each side holds, the base being rebuilt and
// the result being reported.
type reconcile struct {
	dataDir string
	path    string
	local   FileEntry
	remote  FileEntry
	next    map[string]string
	res     *Result
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
		r := reconcile{dataDir: dataDir, path: p, local: local[p], remote: remote[p], next: next, res: res}
		if err := c.reconcilePath(r, base[p]); err != nil {
			return nil, err
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

// reconcilePath settles one path against the checksum it had at the last sync.
// A side matching base did not change; when only one side moved, its change is
// applied outright, and when both moved the pair has either converged on the
// same content or is a genuine conflict.
func (c *Client) reconcilePath(r reconcile, baseChecksum string) error {
	localMod := r.local.Checksum != baseChecksum
	remoteMod := r.remote.Checksum != baseChecksum

	switch {
	case !localMod && !remoteMod:
		setBase(r.next, r.path, r.local.Checksum)
		return nil
	case localMod && !remoteMod:
		return c.applyLocalChange(r)
	case !localMod && remoteMod:
		return c.applyRemoteChange(r)
	}

	if r.local.Checksum == r.remote.Checksum {
		setBase(r.next, r.path, r.local.Checksum)
		return nil
	}
	return c.resolveConflict(r)
}

// applyLocalChange pushes a change only this machine made: an upload, or a
// deletion propagated to the server.
func (c *Client) applyLocalChange(r reconcile) error {
	if r.local.Checksum == "" {
		if err := c.Delete(r.path); err != nil {
			return err
		}
		r.res.DeletedRemote = append(r.res.DeletedRemote, r.path)
		delete(r.next, r.path)
		return nil
	}
	if err := c.uploadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Uploaded = append(r.res.Uploaded, r.path)
	r.next[r.path] = r.local.Checksum
	return nil
}

// applyRemoteChange pulls a change only the server has: a download, or a
// deletion propagated to this machine.
func (c *Client) applyRemoteChange(r reconcile) error {
	if r.remote.Checksum == "" {
		if err := removeLocal(r.dataDir, r.path); err != nil {
			return err
		}
		r.res.DeletedLocal = append(r.res.DeletedLocal, r.path)
		delete(r.next, r.path)
		return nil
	}
	if err := c.downloadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Downloaded = append(r.res.Downloaded, r.path)
	r.next[r.path] = r.remote.Checksum
	return nil
}
