package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/reports"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

type ReportSummary struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Machine string    `json:"machine"`
	Format  string    `json:"format"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
	Expired bool      `json:"expired"`
}

type ReportDetail struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Machine string    `json:"machine"`
	Format  string    `json:"format"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires,omitempty"`
	Expired bool      `json:"expired"`
	Content string    `json:"content"`
}

func reportFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".html" || ext == ".htm" {
		return "html"
	}
	return "markdown"
}

func (s *Server) reportsList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	all, err := reports.List(root)
	if err != nil {
		s.Log.Error("reports list failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	now := time.Now()
	var out []ReportSummary
	for _, rep := range all {
		out = append(out, ReportSummary{
			ID:      rep.ID,
			Title:   rep.Title,
			Machine: rep.Machine,
			Format:  reportFormat(rep.Path),
			Created: rep.Created,
			Expires: rep.Expires,
			Expired: rep.Expired(now),
		})
	}
	if out == nil {
		out = []ReportSummary{}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) reportGet(w http.ResponseWriter, r *http.Request) {
	id, ok := safeName(pathParam(r, "id"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid report id"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	rep, err := reports.Find(root, id)
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	raw, err := os.ReadFile(rep.Path)
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	now := time.Now()
	httpjson.WriteJSON(w, http.StatusOK, ReportDetail{
		ID:      rep.ID,
		Title:   rep.Title,
		Machine: rep.Machine,
		Format:  reportFormat(rep.Path),
		Created: rep.Created,
		Expires: rep.Expires,
		Expired: rep.Expired(now),
		Content: string(raw),
	})
}

func (s *Server) reportDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := safeName(pathParam(r, "id"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid report id"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	if err := reports.Remove(root, id); err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

