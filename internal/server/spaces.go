package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

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
