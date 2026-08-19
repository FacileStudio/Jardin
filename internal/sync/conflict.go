package sync

import (
	"os"
	"path/filepath"
)

// resolveConflict handles a path where both sides changed since base. Content
// always beats a deletion, so nothing is lost; a single-writer path takes the
// fresher copy outright; between two genuine edits it picks a deterministic
// winner and keeps the loser as <path>.conflict.
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
	r.res.Conflicts = append(r.res.Conflicts, r.path+" (edited on both — kept "+r.path+conflictExt+" backup)")
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
// server's version is fetched and parked as <path>.conflict before the local
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
// file is copied to <path>.conflict before being overwritten by the download.
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
