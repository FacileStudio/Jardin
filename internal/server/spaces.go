package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
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

const (
	roleOwner  = "owner"
	roleAdmin  = "admin"
	roleMember = "member"
)

// Space is a named sync scope with its member-to-role map.
type Space struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Members     map[string]string `json:"members"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// SpaceResponse is the space as a member sees it, with their own role.
type SpaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MemberResponse is one member of a space.
type MemberResponse struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

func validRole(role string) bool {
	return role == roleOwner || role == roleAdmin || role == roleMember
}

func newSpaceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *Server) spacesPath() string {
	return filepath.Join(s.DataDir, ".spaces.json")
}

func (s *Server) spaceDir(id string) string {
	return filepath.Join(s.DataDir, "spaces", id)
}

func (s *Server) loadSpaces() map[string]*Space {
	spaces := make(map[string]*Space)
	data, err := os.ReadFile(s.spacesPath())
	if err != nil {
		return spaces
	}
	if err := json.Unmarshal(data, &spaces); err != nil {
		s.Log.Error("spaces: corrupt store", slog.String("path", s.spacesPath()), slog.Any("error", err))
		return make(map[string]*Space)
	}
	return spaces
}

func (s *Server) saveSpaces(spaces map[string]*Space) error {
	data, err := json.MarshalIndent(spaces, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.spacesPath(), data, 0o600)
}

// spaceAccess is the guard every space_id-accepting route funnels through: a
// caller-supplied space id is untrusted input, so membership (or admin scope)
// is checked before the id is ever used as a path component.
func (s *Server) spaceAccess(id Identity, spaceID string) (*Space, string, bool) {
	s.mu.RLock()
	space, ok := s.loadSpaces()[spaceID]
	s.mu.RUnlock()
	if !ok {
		return nil, "", false
	}
	if id.Scope == scopeAdmin {
		role := space.Members[id.Email]
		if role == "" {
			role = roleAdmin
		}
		return space, role, true
	}
	if id.Email == "" {
		return nil, "", false
	}
	role, member := space.Members[id.Email]
	if !member {
		return nil, "", false
	}
	return space, role, true
}

func spaceResponse(space *Space, role string) SpaceResponse {
	return SpaceResponse{
		ID: space.ID, Name: space.Name, Description: space.Description,
		Role: role, CreatedAt: space.CreatedAt, UpdatedAt: space.UpdatedAt,
	}
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id := identityFrom(r)
	if id.Email == "" && id.Scope != scopeAdmin {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return id, false
	}
	return id, true
}

func (s *Server) spacesList(w http.ResponseWriter, r *http.Request) {
	id := identityFrom(r)
	s.mu.RLock()
	spaces := s.loadSpaces()
	s.mu.RUnlock()
	out := []SpaceResponse{}
	for _, space := range spaces {
		if role, ok := space.Members[id.Email]; ok && id.Email != "" {
			out = append(out, spaceResponse(space, role))
		} else if id.Scope == scopeAdmin {
			out = append(out, spaceResponse(space, roleAdmin))
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
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		httpjson.WriteError(w, apierrors.Invalid("email required"))
		return
	}
	if req.Role == "" {
		req.Role = roleMember
	}
	if !validRole(req.Role) {
		httpjson.WriteError(w, apierrors.Invalid("invalid role"))
		return
	}
	if req.Role == roleOwner && role != roleOwner {
		httpjson.WriteError(w, apierrors.Forbidden("only owners can grant owner"))
		return
	}
	s.mu.Lock()
	if _, exists := s.loadUsers()[req.Email]; !exists {
		s.mu.Unlock()
		httpjson.WriteError(w, apierrors.Invalid("unknown user"))
		return
	}
	spaces := s.loadSpaces()
	if sp := spaces[space.ID]; sp != nil {
		sp.Members[req.Email] = req.Role
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
