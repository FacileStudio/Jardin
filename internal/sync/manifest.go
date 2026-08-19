package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type manifest struct {
	SyncedAt string            `json:"synced_at"`
	Files    map[string]string `json:"files"`
}

func manifestPath(dataDir string) string {
	return filepath.Join(dataDir, manifestName)
}

// loadManifest reads the last-synced base. A corrupt manifest must not block
// sync, so it is rebuilt from scratch and treated as an empty base.
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
