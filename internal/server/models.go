package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// ModelInfo is one typed step extension found under extensions/models. Type is
// the dotted form a flow's `type:` field uses; Path is where it lives relative
// to the models root, which is what /models/{path} reads.
type ModelInfo struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// modelsRoot resolves a scope's models directory. It is a subdirectory, not
// its own scope, because models sync and trust exactly like flows do — one
// tree, same rules.
func modelsRoot(root string) string {
	return filepath.Join(root, "extensions", "models")
}

// modelsList walks the models tree, skipping _lib — shared helper code a
// model imports, not a model itself.
func (s *Server) modelsList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	base := modelsRoot(root)
	var models []ModelInfo
	filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".ts") {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "_lib" || strings.HasPrefix(rel, "_lib/") {
			return nil
		}
		models = append(models, ModelInfo{Type: "@" + strings.TrimSuffix(rel, ".ts"), Path: rel})
		return nil
	})
	if models == nil {
		models = []ModelInfo{}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Type < models[j].Type })
	httpjson.WriteJSON(w, http.StatusOK, models)
}

// modelGet reads a model's raw TypeScript source. jardin never runs it
// server-side — the container ships no bun and no shell — so there is no
// schema to answer with, only the file a Rule-style viewer can show.
func (s *Server) modelGet(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(modelsRoot(root), pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	http.ServeFile(w, r, full)
}
