package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Mycelium/internal/memory"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// StatusResponse is the /api/status payload: machine identity plus the rule
// and skill names that apply to it.
type StatusResponse struct {
	Machine string   `json:"machine"`
	Rules   []string `json:"rules"`
	Skills  []string `json:"skills"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, StatusResponse{
		Rules:  listMdNames(filepath.Join(root, "rules")),
		Skills: listMdNames(filepath.Join(root, "skills")),
	})
}

func (s *Server) memorySearch(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		httpjson.WriteJSON(w, http.StatusOK, []memory.SearchResult{})
		return
	}
	results, err := memory.Search(filepath.Join(root, "memory"), query)
	if err != nil {
		s.Log.Error("memory search failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if results == nil {
		results = []memory.SearchResult{}
	}
	httpjson.WriteJSON(w, http.StatusOK, results)
}

func (s *Server) memoryIndex(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	content, err := memory.ReadIndex(filepath.Join(root, "memory"))
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, listMdNames(filepath.Join(root, "rules")))
}

func (s *Server) ruleGet(w http.ResponseWriter, r *http.Request) {
	s.readNamed(w, r, "rules")
}

func (s *Server) skillGet(w http.ResponseWriter, r *http.Request) {
	s.readNamed(w, r, "skills")
}

func (s *Server) ruleSave(w http.ResponseWriter, r *http.Request) {
	s.writeNamed(w, r, "rules")
}

func (s *Server) skillSave(w http.ResponseWriter, r *http.Request) {
	s.writeNamed(w, r, "skills")
}

func (s *Server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	s.deleteNamed(w, r, "rules")
}

func (s *Server) skillDelete(w http.ResponseWriter, r *http.Request) {
	s.deleteNamed(w, r, "skills")
}

func (s *Server) readNamed(w http.ResponseWriter, r *http.Request, kind string) {
	name, ok := safeName(pathParam(r, "name"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid name"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	data, err := os.ReadFile(filepath.Join(root, kind, name+".md"))
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) writeNamed(w http.ResponseWriter, r *http.Request, kind string) {
	name, ok := safeName(pathParam(r, "name"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid name"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	if err := writeFile(filepath.Join(root, kind), name+".md", r.Body); err != nil {
		s.Log.Error("save failed", slog.String("kind", kind), slog.String("name", name), slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteNamed(w http.ResponseWriter, r *http.Request, kind string) {
	name, ok := safeName(pathParam(r, "name"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid name"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	os.Remove(filepath.Join(root, kind, name+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillsList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, listMdNames(filepath.Join(root, "skills")))
}
