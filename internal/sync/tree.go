package sync

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// syncSkip reports whether a path is excluded from sync: the tokens file,
// hidden dotfiles, conflict backups, logs and flow run artifacts never travel.
func syncSkip(rel string) bool {
	return rel == tokensFile ||
		strings.HasPrefix(rel, ".") ||
		strings.HasPrefix(rel, "runs/") ||
		strings.HasSuffix(rel, conflictExt) ||
		strings.HasSuffix(rel, ".log")
}

// LocalTree walks the data directory and returns every file in it as a
// FileEntry.
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
