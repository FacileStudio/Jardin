package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// FileEntry is one syncable file's identity over the wire.
type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

func (s *Server) syncTree(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	var files []FileEntry
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if syncSkip(rel) {
			return nil
		}
		data, _ := os.ReadFile(path)
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		files = append(files, FileEntry{
			Path:     rel,
			Checksum: checksum,
			Size:     info.Size(),
			ModTime:  info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if files == nil {
		files = []FileEntry{}
	}
	httpjson.WriteJSON(w, http.StatusOK, files)
}

func (s *Server) syncGetFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	http.ServeFile(w, r, full)
}

func (s *Server) syncPutFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		s.Log.Error("sync put: mkdir failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		s.Log.Error("sync put: write failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	s.enqueueEmbed(root, full)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncDeleteFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		s.Log.Error("sync delete failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	s.enqueueEmbed(root, full)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveSyncPath(root, rel string) (string, bool) {
	clean := strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	if clean == "." || syncSkip(clean) {
		return "", false
	}
	full := filepath.Join(root, clean)
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// syncSkip fences everything the file sync must never touch: server state
// (tokens, dotfiles), conflict backups, and the spaces subtree — space content
// is reachable only through its own scoped root, never via the common tree.
func syncSkip(rel string) bool {
	return rel == "tokens.json" ||
		strings.HasPrefix(rel, ".") ||
		strings.HasPrefix(rel, "runs/") ||
		strings.HasSuffix(rel, ".conflict") ||
		rel == "spaces" || strings.HasPrefix(rel, "spaces"+string(os.PathSeparator)) || strings.HasPrefix(rel, "spaces/")
}
