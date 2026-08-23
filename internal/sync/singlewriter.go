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
func (c *Client) resolveSingleWriter(r reconcile) error {
	if localWins(r.local, r.remote) {
		if err := c.uploadFile(r.dataDir, r.path); err != nil {
			return err
		}
		r.res.Uploaded = append(r.res.Uploaded, r.path)
		r.next[r.path] = r.local.Checksum
		return nil
	}
	if err := c.downloadFile(r.dataDir, r.path); err != nil {
		return err
	}
	r.res.Downloaded = append(r.res.Downloaded, r.path)
	r.next[r.path] = r.remote.Checksum
	return nil
}

// pruneSingleWriterConflicts deletes conflict backups on single-writer paths.
// They are this machine's own stale telemetry, two snapshots of one writer that
// differ by a timestamp, and nothing else ever removes them: they are excluded
// from sync, so they sit on disk keeping "mycelium doctor" red forever.
//
// This runs on every sync rather than only when a conflict is resolved. Once
// conflicts on these paths are prevented, the resolve path stops being reached,
// so cleaning up there would only ever heal a machine that was still broken.
//
// Both candidate locations are removed for each page, because a machine can be
// carrying a backup written before the move to .conflicts/ and one written
// after, and only the layout tells them apart.
func pruneSingleWriterConflicts(dataDir string) error {
	for _, page := range ConflictBackups(dataDir) {
		if !singleWriter(page) {
			continue
		}
		legacy := filepath.Join(dataDir, filepath.FromSlash(page)) + conflictExt
		for _, candidate := range []string{conflictPath(dataDir, page), legacy} {
			if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
