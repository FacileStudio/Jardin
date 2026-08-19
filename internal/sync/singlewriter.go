package sync

import (
	"os"
	"path/filepath"
	"strings"
)

// singleWriter reports whether a path lives in a directory only one machine ever
// writes to. usage/<machine>/ holds that machine's own telemetry, rewritten by
// its status line every few seconds, so both sides differing there is one writer
// outrunning its last sync — never two machines disagreeing. Merging it as if it
// were a shared file produces a conflict on every pass that no one can resolve,
// because the two copies differ only by a timestamp.
func singleWriter(rel string) bool {
	rest, ok := strings.CutPrefix(rel, usagePrefix)
	return ok && strings.Contains(rest, "/")
}

// resolveSingleWriter settles a both-sides difference on a path with one writer.
// There is nothing to merge and no second author to preserve, so the fresher copy
// simply wins: no backup is kept, and the pair is not reported as a conflict.
//
// It also clears any backup an earlier version left behind. Those files are this
// machine's own stale telemetry, they are never synced, and nothing else removes
// them — so without this, one machine that hit the old behaviour keeps reporting
// a conflict in "jardin doctor" forever.
func (c *Client) resolveSingleWriter(dataDir, p string, local, remote FileEntry, next map[string]string, res *Result) error {
	if err := removeConflictCopy(dataDir, p); err != nil {
		return err
	}
	if localWins(local, remote) {
		if err := c.uploadFile(dataDir, p); err != nil {
			return err
		}
		res.Uploaded = append(res.Uploaded, p)
		next[p] = local.Checksum
		return nil
	}
	if err := c.downloadFile(dataDir, p); err != nil {
		return err
	}
	res.Downloaded = append(res.Downloaded, p)
	next[p] = remote.Checksum
	return nil
}

func removeConflictCopy(dataDir, p string) error {
	err := os.Remove(filepath.Join(dataDir, filepath.FromSlash(p)) + conflictExt)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
