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
func (c *Client) resolveSingleWriter(dataDir, p string, local, remote FileEntry, next map[string]string, res *Result) error {
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

// pruneSingleWriterConflicts deletes conflict backups on single-writer paths.
// They are this machine's own stale telemetry, two snapshots of one writer that
// differ by a timestamp, and nothing else ever removes them: they are excluded
// from sync, so they sit on disk keeping "jardin doctor" red forever.
//
// This runs on every sync rather than only when a conflict is resolved. Once
// conflicts on these paths are prevented, the resolve path stops being reached —
// so cleaning up there would only ever heal a machine that was still broken.
func pruneSingleWriterConflicts(dataDir string) error {
	root := filepath.Join(dataDir, filepath.FromSlash(usagePrefix))
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, conflictExt) {
			return nil
		}
		rel, relErr := filepath.Rel(dataDir, path)
		if relErr != nil {
			return nil
		}
		if !singleWriter(strings.TrimSuffix(filepath.ToSlash(rel), conflictExt)) {
			return nil
		}
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return rmErr
		}
		return nil
	})
}
