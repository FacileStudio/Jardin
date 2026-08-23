package sync

import "fmt"

const (
	manifestName = ".sync-base.json"
	conflictExt  = ".conflict"
	conflictDir  = ".conflicts"
	tokensFile   = "tokens.json"
	usagePrefix  = "usage/"
)

// MaxSilentDeletes is how many files one reconcile may destroy on its own,
// counting both directions. On 2026-08-19 a reconcile removed 246 pages and
// nothing could bring them back, so a plan bigger than this has to be confirmed
// by a human instead of running unattended from the daemon. It is exported so
// the flag that waives it can quote the same number rather than a copy.
const MaxSilentDeletes = 10

// BulkDeleteError reports a reconcile that was stopped before it destroyed more
// files than MaxSilentDeletes allows. The two directions are kept apart because
// losing pages from this machine and losing them from the server every other
// machine pulls from are different accidents. Nothing was written either way:
// no file was deleted on either side, nothing was uploaded, and the base was
// left as it was.
type BulkDeleteError struct {
	Local  []string
	Remote []string
}

// Total is how many files the refused reconcile would have destroyed, which is
// the number the limit is measured against.
func (e *BulkDeleteError) Total() int {
	return len(e.Local) + len(e.Remote)
}

// Error states the refusal in the terms a human reads on the command line.
func (e *BulkDeleteError) Error() string {
	switch {
	case len(e.Remote) == 0:
		return fmt.Sprintf("sync stopped: %d files are gone from the server and would be deleted here", len(e.Local))
	case len(e.Local) == 0:
		return fmt.Sprintf("sync stopped: %d files were removed here and would be deleted on the server", len(e.Remote))
	default:
		return fmt.Sprintf("sync stopped: %d files would be deleted here and %d on the server",
			len(e.Local), len(e.Remote))
	}
}

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
// which path is being decided, what each side holds, the checksum the path had
// at the last sync, the base being rebuilt and the result being reported.
type reconcile struct {
	dataDir string
	path    string
	local   FileEntry
	remote  FileEntry
	base    string
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

	paths := unionPaths(local, remote, base)
	plan := plannedDeletes(paths, local, remote, base)
	if plan.Total() > MaxSilentDeletes && !c.AllowBulkDelete {
		return nil, plan
	}

	next := make(map[string]string, len(base))
	for k, v := range base {
		next[k] = v
	}

	res := &Result{}
	for _, p := range paths {
		r := reconcile{
			dataDir: dataDir, path: p, next: next, res: res,
			local: local[p], remote: remote[p], base: base[p],
		}
		if err := c.reconcilePath(r); err != nil {
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

// plannedDeletes lists every file this reconcile would destroy, in the order it
// will visit them, which unionPaths has already sorted. It reads the same maps
// the loop does, so the whole plan is known before the first file is touched
// and a refusal can leave both trees exactly as it found them. The plan is
// returned as the error it would become, so the refusal and the plan cannot
// end up describing different things.
func plannedDeletes(paths []string, local, remote map[string]FileEntry, base map[string]string) *BulkDeleteError {
	plan := &BulkDeleteError{}
	for _, p := range paths {
		switch {
		case deletesLocal(local[p], remote[p], base[p]):
			plan.Local = append(plan.Local, p)
		case deletesRemote(local[p], remote[p], base[p]):
			plan.Remote = append(plan.Remote, p)
		}
	}
	return plan
}

// deletesLocal reports whether settling this path removes the local file: this
// machine has not touched it since the last sync, the server has, and what the
// server now holds is nothing. Both the guard and applyRemoteChange ask this
// question, and they must not be able to answer it differently.
func deletesLocal(local, remote FileEntry, baseChecksum string) bool {
	return local.Checksum == baseChecksum &&
		remote.Checksum != baseChecksum &&
		remote.Checksum == ""
}

// deletesRemote is the same question in the other direction: the server has not
// touched this path since the last sync, this machine has, and what this
// machine now holds is nothing. This is the direction that empties the copy
// every other machine pulls from, so it counts against the same limit.
func deletesRemote(local, remote FileEntry, baseChecksum string) bool {
	return remote.Checksum == baseChecksum &&
		local.Checksum != baseChecksum &&
		local.Checksum == ""
}

// reconcilePath settles one path against the checksum it had at the last sync.
// A side matching base did not change; when only one side moved, its change is
// applied outright, and when both moved the pair has either converged on the
// same content or is a genuine conflict.
func (c *Client) reconcilePath(r reconcile) error {
	localMod := r.local.Checksum != r.base
	remoteMod := r.remote.Checksum != r.base

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
	if deletesRemote(r.local, r.remote, r.base) {
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
	if deletesLocal(r.local, r.remote, r.base) {
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
