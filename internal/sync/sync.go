package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	manifestName = ".sync-base.json"
	conflictExt  = ".conflict"
	tokensFile   = "tokens.json"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

func (c *Client) Tree() ([]FileEntry, error) {
	resp, err := c.do("GET", "/api/sync/tree", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tree: %s", resp.Status)
	}
	var entries []FileEntry
	return entries, json.NewDecoder(resp.Body).Decode(&entries)
}

func (c *Client) Download(filePath string) ([]byte, error) {
	resp, err := c.do("GET", "/api/sync/files/"+filePath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", filePath, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) Upload(filePath string, data []byte) error {
	resp, err := c.do("PUT", "/api/sync/files/"+filePath, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("upload %s: %s", filePath, resp.Status)
	}
	return nil
}

func (c *Client) Delete(filePath string) error {
	resp, err := c.do("DELETE", "/api/sync/files/"+filePath, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete %s: %s", filePath, resp.Status)
	}
	return nil
}

func syncSkip(rel string) bool {
	return rel == tokensFile ||
		strings.HasPrefix(rel, ".") ||
		strings.HasSuffix(rel, conflictExt)
}

func LocalTree(dataDir string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dataDir, path)
		rel = filepath.ToSlash(rel)
		if syncSkip(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		entries = append(entries, FileEntry{
			Path:     rel,
			Checksum: checksum(data),
			Size:     info.Size(),
			ModTime:  info.ModTime().UTC().Format(time.RFC3339Nano),
		})
		return nil
	})
	return entries, err
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

// Sync reconciles local and remote against the last-synced base manifest.
// Local-only changes are pushed, remote-only changes are pulled, deletions
// propagate both ways, and genuine conflicts pick a deterministic winner while
// preserving the loser as a sibling ".conflict" file. It never silently
// overwrites a local edit.
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

		default: // both sides changed since base
			if lc == rc { // same change made on both — already converged
				setBase(next, p, lc)
				continue
			}
			if err := c.resolveConflict(dataDir, p, local[p], remote[p], next, res); err != nil {
				return nil, err
			}
		}
	}

	if err := saveManifest(dataDir, next); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) resolveConflict(dataDir, p string, local, remote FileEntry, next map[string]string, res *Result) error {
	// Delete-vs-edit: content always beats a deletion, so nothing is lost.
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

	// Edit-vs-edit: deterministic winner, loser backed up to <path>.conflict.
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

func writeConflictCopy(dataDir, p string, data []byte) error {
	full := filepath.Join(dataDir, filepath.FromSlash(p)) + conflictExt
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
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

type manifest struct {
	SyncedAt string            `json:"synced_at"`
	Files    map[string]string `json:"files"`
}

func manifestPath(dataDir string) string {
	return filepath.Join(dataDir, manifestName)
}

func loadManifest(dataDir string) (map[string]string, error) {
	data, err := os.ReadFile(manifestPath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		// A corrupt manifest must not block sync; rebuild it from scratch.
		return map[string]string{}, nil
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m.Files, nil
}

func saveManifest(dataDir string, files map[string]string) error {
	m := manifest{SyncedAt: time.Now().UTC().Format(time.RFC3339), Files: files}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath(dataDir), data, 0o600)
}

func indexByPath(entries []FileEntry) map[string]FileEntry {
	m := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		m[e.Path] = e
	}
	return m
}

func unionPaths(maps ...interface{}) []string {
	seen := map[string]struct{}{}
	for _, m := range maps {
		switch t := m.(type) {
		case map[string]FileEntry:
			for k := range t {
				seen[k] = struct{}{}
			}
		case map[string]string:
			for k := range t {
				seen[k] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func setBase(base map[string]string, p, checksum string) {
	if checksum == "" {
		delete(base, p)
		return
	}
	base[p] = checksum
}

func checksum(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
