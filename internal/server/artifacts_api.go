package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/artifacts"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// ArtifactSummary represents the metadata of an artifact for API listings.
type ArtifactSummary struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Machine string    `json:"machine"`
	Format  string    `json:"format"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
	Expired bool      `json:"expired"`
}

// ReportSummary is a backward-compatible alias for ArtifactSummary.
type ReportSummary = ArtifactSummary

// ArtifactDetail represents the full content and metadata of an artifact.
type ArtifactDetail struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Machine string    `json:"machine"`
	Format  string    `json:"format"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
	Expired bool      `json:"expired"`
	Content string    `json:"content"`
}

// ReportDetail is a backward-compatible alias for ArtifactDetail.
type ReportDetail = ArtifactDetail

func artifactFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".html" || ext == ".htm" {
		return "html"
	}
	return "markdown"
}

func (s *Server) artifactsList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	all, err := artifacts.List(root)
	if err != nil {
		s.Log.Error("artifacts list failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	now := time.Now()
	var out []ArtifactSummary
	for _, art := range all {
		out = append(out, ArtifactSummary{
			ID:      art.ID,
			Title:   art.Title,
			Machine: art.Machine,
			Format:  artifactFormat(art.Path),
			Created: art.Created,
			Expires: art.Expires,
			Expired: art.Expired(now),
		})
	}
	if out == nil {
		out = []ArtifactSummary{}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) artifactGet(w http.ResponseWriter, r *http.Request) {
	id, ok := safeName(pathParam(r, "id"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid artifact id"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	art, err := artifacts.Find(root, id)
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	raw, err := os.ReadFile(art.Path)
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	now := time.Now()
	httpjson.WriteJSON(w, http.StatusOK, ArtifactDetail{
		ID:      art.ID,
		Title:   art.Title,
		Machine: art.Machine,
		Format:  artifactFormat(art.Path),
		Created: art.Created,
		Expires: art.Expires,
		Expired: art.Expired(now),
		Content: string(raw),
	})
}

func (s *Server) artifactDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := safeName(pathParam(r, "id"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid artifact id"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	if err := artifacts.Remove(root, id); err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reportsList(w http.ResponseWriter, r *http.Request) {
	s.artifactsList(w, r)
}

func (s *Server) reportGet(w http.ResponseWriter, r *http.Request) {
	s.artifactGet(w, r)
}

func (s *Server) reportDelete(w http.ResponseWriter, r *http.Request) {
	s.artifactDelete(w, r)
}
