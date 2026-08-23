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

// skipWalkDir reports whether a whole directory can be pruned from a walk
// rather than filtered one file at a time. The names are the same ones
// syncSkip already excludes, asked one level up: a trailing slash is what makes
// "runs" match the "runs/" prefix rule.
//
// The root is never skipped. Its relative path is ".", which the dotfile rule
// would match, and pruning it would return an empty tree that the reconciler
// reads as every file having been deleted.
func skipWalkDir(rel string) bool {
	return rel != "." && syncSkip(rel+"/")
}

// LocalTree walks the data directory and returns every file in it as a
// FileEntry. Excluded directories are pruned rather than walked: returning nil
// for .git still descends into it and stats every object before discarding
// each one, and that cost grows with every commit the journal will add.
func LocalTree(dataDir string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dataDir, path)
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if skipWalkDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
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
