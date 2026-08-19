package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/httpjson"
)

func (s *Server) spacesMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	space, _, ok := s.spaceAccess(id, r.PathValue("id"))
	if !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	s.mu.RLock()
	users := s.loadUsers()
	s.mu.RUnlock()
	out := []MemberResponse{}
	for email, role := range space.Members {
		out = append(out, MemberResponse{Email: email, Name: users[email].Name, Role: role, JoinedAt: space.CreatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	httpjson.WriteJSON(w, http.StatusOK, map[string]any{"members": out})
}

// decodeMemberGrant reads the body of a grant and settles what role it asks
// for, defaulting to member. Only an owner may create another owner, so that
// check lives here with the role it is about rather than at the call site.
func decodeMemberGrant(w http.ResponseWriter, r *http.Request, actorRole string) (string, string, bool) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		httpjson.WriteError(w, apierrors.Invalid("email required"))
		return "", "", false
	}
	if req.Role == "" {
		req.Role = roleMember
	}
	if !validRole(req.Role) {
		httpjson.WriteError(w, apierrors.Invalid("invalid role"))
		return "", "", false
	}
	if req.Role == roleOwner && actorRole != roleOwner {
		httpjson.WriteError(w, apierrors.Forbidden("only owners can grant owner"))
		return "", "", false
	}
	return req.Email, req.Role, true
}

func (s *Server) spacesMemberAdd(w http.ResponseWriter, r *http.Request) {
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
	email, granted, ok := decodeMemberGrant(w, r, role)
	if !ok {
		return
	}
	s.mu.Lock()
	if _, exists := s.loadUsers()[email]; !exists {
		s.mu.Unlock()
		httpjson.WriteError(w, apierrors.Invalid("unknown user"))
		return
	}
	spaces := s.loadSpaces()
	if sp := spaces[space.ID]; sp != nil {
		sp.Members[email] = granted
		sp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	err := s.saveSpaces(spaces)
	s.mu.Unlock()
	if err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) spacesMemberUpdate(w http.ResponseWriter, r *http.Request) {
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
	email := r.PathValue("email")
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validRole(req.Role) {
		httpjson.WriteError(w, apierrors.Invalid("invalid role"))
		return
	}
	s.mu.Lock()
	spaces := s.loadSpaces()
	sp := spaces[space.ID]
	if sp == nil || sp.Members[email] == "" {
		s.mu.Unlock()
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	sp.Members[email] = req.Role
	sp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	err := s.saveSpaces(spaces)
	s.mu.Unlock()
	if err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) spacesMemberRemove(w http.ResponseWriter, r *http.Request) {
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
	s.removeMember(w, space.ID, r.PathValue("email"))
}

func (s *Server) spacesLeave(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	space, _, ok := s.spaceAccess(id, r.PathValue("id"))
	if !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	if id.Email == "" {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return
	}
	s.removeMember(w, space.ID, id.Email)
}

func (s *Server) removeMember(w http.ResponseWriter, spaceID, email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spaces := s.loadSpaces()
	sp := spaces[spaceID]
	if sp == nil || sp.Members[email] == "" {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	if sp.Members[email] == roleOwner {
		owners := 0
		for _, r := range sp.Members {
			if r == roleOwner {
				owners++
			}
		}
		if owners <= 1 {
			httpjson.WriteError(w, apierrors.Conflict("cannot remove the only owner; transfer ownership or delete the space"))
			return
		}
	}
	delete(sp.Members, email)
	sp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveSpaces(spaces); err != nil {
		s.Log.Error("spaces: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
