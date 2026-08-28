package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/httpjson"
)

func (s *Server) spacesList(w http.ResponseWriter, r *http.Request) {
	id := identityFrom(r)
	s.mu.RLock()
	spaces := s.loadSpaces()
	s.mu.RUnlock()
	out := []SpaceResponse{}
	for _, space := range spaces {
		if role, ok := space.Members[id.Email]; ok && id.Email != "" {
			out = append(out, spaceResponse(space, role))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	httpjson.WriteJSON(w, http.StatusOK, map[string]any{"spaces": out})
}

func (s *Server) spacesCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpjson.WriteError(w, apierrors.Invalid("name required"))
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	space := &Space{
		ID: newSpaceID(), Name: strings.TrimSpace(req.Name), Description: req.Description,
		Members: map[string]string{}, CreatedAt: now, UpdatedAt: now,
	}
	if id.Email != "" {
		space.Members[id.Email] = roleOwner
	}
	s.mu.Lock()
	spaces := s.loadSpaces()
	spaces[space.ID] = space
	err := s.saveSpaces(spaces)
	s.mu.Unlock()
	if err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	for _, d := range []string{"memory", "rules", "skills", "sessions"} {
		os.MkdirAll(filepath.Join(s.spaceDir(space.ID), d), 0o755)
	}
	httpjson.WriteJSON(w, http.StatusCreated, spaceResponse(space, roleOwner))
}

func (s *Server) spacesUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	space, role, ok := s.spaceAccess(id, r.PathValue("id"))
	if !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	if role != roleOwner && role != roleAdmin {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		httpjson.WriteError(w, apierrors.Invalid("name required"))
		return
	}
	s.mu.Lock()
	spaces := s.loadSpaces()
	if sp := spaces[space.ID]; sp != nil {
		sp.Name = strings.TrimSpace(req.Name)
		sp.Description = req.Description
		sp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		space = sp
	}
	err := s.saveSpaces(spaces)
	s.mu.Unlock()
	if err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, spaceResponse(space, role))
}

func (s *Server) spacesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	space, role, ok := s.spaceAccess(id, r.PathValue("id"))
	if !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	if role != roleOwner {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return
	}
	s.mu.Lock()
	spaces := s.loadSpaces()
	delete(spaces, space.ID)
	err := s.saveSpaces(spaces)
	s.mu.Unlock()
	if err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if _, statErr := os.Stat(s.spaceDir(space.ID)); statErr == nil {
		trash := filepath.Join(s.DataDir, ".trash")
		os.MkdirAll(trash, 0o755)
		os.Rename(s.spaceDir(space.ID), filepath.Join(trash, space.ID+"-"+time.Now().UTC().Format("20060102T150405")))
	}
	w.WriteHeader(http.StatusNoContent)
}
