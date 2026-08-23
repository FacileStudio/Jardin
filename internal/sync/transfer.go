package sync

import (
	"os"
	"path/filepath"
)

func (c *Client) uploadFile(dataDir, p string) error {
	data, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(p)))
	if err != nil {
		return err
	}
	return c.Upload(p, data)
}

func (c *Client) downloadFile(dataDir, p string) error {
	data, err := c.Download(p)
	if err != nil {
		return err
	}
	full := filepath.Join(dataDir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func removeLocal(dataDir, p string) error {
	if err := os.Remove(filepath.Join(dataDir, filepath.FromSlash(p))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Push forces local files up, overwriting the server. One-directional escape
// hatch; advances the base for everything it uploads.
func (c *Client) Push(dataDir string) (*Result, error) {
	local, err := LocalTree(dataDir)
	if err != nil {
		return nil, err
	}
	remote, err := c.Tree()
	if err != nil {
		return nil, err
	}
	remoteMap := indexByPath(remote)
	base, err := loadManifest(dataDir)
	if err != nil {
		return nil, err
	}

	res := &Result{}
	for _, l := range local {
		if r, ok := remoteMap[l.Path]; !ok || l.Checksum != r.Checksum {
			if err := c.uploadFile(dataDir, l.Path); err != nil {
				return nil, err
			}
			res.Uploaded = append(res.Uploaded, l.Path)
		}
		base[l.Path] = l.Checksum
	}
	if err := saveManifest(dataDir, base); err != nil {
		return nil, err
	}
	return res, nil
}

// Pull forces remote files down, overwriting local. One-directional escape
// hatch; advances the base for everything it downloads.
func (c *Client) Pull(dataDir string) (*Result, error) {
	local, err := LocalTree(dataDir)
	if err != nil {
		return nil, err
	}
	localMap := indexByPath(local)
	remote, err := c.Tree()
	if err != nil {
		return nil, err
	}
	base, err := loadManifest(dataDir)
	if err != nil {
		return nil, err
	}

	res := &Result{}
	for _, r := range remote {
		if l, ok := localMap[r.Path]; !ok || r.Checksum != l.Checksum {
			if err := c.downloadFile(dataDir, r.Path); err != nil {
				return nil, err
			}
			res.Downloaded = append(res.Downloaded, r.Path)
		}
		base[r.Path] = r.Checksum
	}
	if err := saveManifest(dataDir, base); err != nil {
		return nil, err
	}
	return res, nil
}

// ResetBase discards the last-synced base manifest. Callers use it when the
// remote tree identity changes (switching spaces), where a stale base would
// make the reconciler propagate bogus deletions; the next Sync then performs a
// non-destructive union merge instead.
func ResetBase(dataDir string) error {
	if err := os.Remove(manifestPath(dataDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
