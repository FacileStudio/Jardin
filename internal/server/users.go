package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// User is an account on the server.
type User struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	Admin     bool   `json:"admin"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) usersPath() string {
	return filepath.Join(s.DataDir, ".users.json")
}

func (s *Server) loadUsers() map[string]User {
	users := make(map[string]User)
	data, err := os.ReadFile(s.usersPath())
	if err != nil {
		return users
	}
	if err := json.Unmarshal(data, &users); err != nil {
		s.Log.Error("users: corrupt store", slog.String("path", s.usersPath()), slog.Any("error", err))
		return make(map[string]User)
	}
	return users
}

func (s *Server) saveUsers(users map[string]User) error {
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.usersPath(), data, 0o600)
}

// upsertUser creates or refreshes a user keyed by email. The first user ever
// seen becomes admin, mirroring the Nuage bootstrap convention.
func (s *Server) upsertUser(email, name string) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := s.loadUsers()
	user, ok := users[email]
	if !ok {
		user = User{
			Email:     email,
			Admin:     len(users) == 0,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}
	if name != "" {
		user.Name = name
	}
	users[email] = user
	if err := s.saveUsers(users); err != nil {
		s.Log.Error("users: save failed", slog.Any("error", err))
	}
	return user
}

func (s *Server) usersList(w http.ResponseWriter, r *http.Request) {
	id := identityFrom(r)
	if id.Email == "" && id.Scope != scopeAdmin {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return
	}
	s.mu.RLock()
	users := s.loadUsers()
	s.mu.RUnlock()
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	id := identityFrom(r)
	if id.Email != "" {
		s.mu.RLock()
		user, ok := s.loadUsers()[id.Email]
		s.mu.RUnlock()
		if ok {
			httpjson.WriteJSON(w, http.StatusOK, map[string]any{"email": user.Email, "name": user.Name, "admin": user.Admin})
			return
		}
		httpjson.WriteJSON(w, http.StatusOK, map[string]any{"email": id.Email, "name": "", "admin": id.Scope == scopeAdmin})
		return
	}
	if id.Scope == scopeAdmin {
		httpjson.WriteJSON(w, http.StatusOK, map[string]any{"email": "", "name": "admin", "admin": true})
		return
	}
	httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	id := identityFrom(r)
	if id.TokenHash != "" {
		s.mu.Lock()
		delete(s.tokens, id.TokenHash)
		s.saveTokens()
		s.mu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}
