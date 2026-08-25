package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

// TokenInfo is a minted API token as stored on disk.
type TokenInfo struct {
	Hash      string `json:"-"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	UserEmail string `json:"user_email,omitempty"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	LastSeen  string `json:"last_seen"`
}

func (s *Server) tokensPath() string {
	return filepath.Join(s.DataDir, "tokens.json")
}

func (s *Server) loadTokens() {
	data, err := os.ReadFile(s.tokensPath())
	if err != nil {
		return
	}
	var raw map[string]TokenInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		s.Log.Error("tokens: failed to parse", slog.String("path", s.tokensPath()), slog.Any("error", err))
		return
	}
	tokens := make(map[string]TokenInfo, len(raw))
	migrated := false
	for key, info := range raw {
		hash := key
		if info.Scope == "" {
			hash = hashToken(key)
			if info.Name == "session" {
				info.Scope = scopeAdmin
			} else {
				info.Scope = scopeSync
			}
			migrated = true
		}
		info.Hash = hash
		tokens[hash] = info
	}
	s.tokens = tokens
	if migrated {
		s.saveTokens()
	}
}

func (s *Server) saveTokens() {
	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		s.Log.Error("tokens: marshal failed", slog.Any("error", err))
		return
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		s.Log.Error("tokens: mkdir failed", slog.Any("error", err))
		return
	}
	if err := os.WriteFile(s.tokensPath(), data, 0o600); err != nil {
		s.Log.Error("tokens: write failed", slog.Any("error", err))
	}
}

// mintToken generates a new bearer token, stores its hash under the given
// machine name (replacing any prior token with the same name), and returns the
// plaintext token to hand to the caller exactly once. A non-empty email ties
// the token to a user, which is what lets a machine sync space trees.
func (s *Server) mintToken(name, scope, email string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(token)
	s.mu.Lock()
	for k, v := range s.tokens {
		if v.Name == name {
			delete(s.tokens, k)
		}
	}
	s.tokens[hash] = TokenInfo{
		Hash:      hash,
		Name:      name,
		Scope:     scope,
		UserEmail: email,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.saveTokens()
	s.mu.Unlock()
	return token, nil
}

// isLoginSession reports whether an entry is a browser or CLI login rather
// than a token somebody created and manages.
//
// The test is expiry, not the name. Sessions are the only entries minted with
// one — a machine token does not expire and an API token does not either — so
// this keeps holding when a third kind of login arrives under a fourth name
// prefix. The page that got this wrong compared the whole name against
// "session", which matched none of "session:<email>" or "cli:<email>", so
// every login ever made was listed as an API token.
func isLoginSession(t TokenInfo) bool { return t.ExpiresAt != "" }

func (s *Server) tokensList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]TokenInfo, 0, len(s.tokens))
	for _, t := range s.tokens {
		if isLoginSession(t) {
			continue
		}
		list = append(list, TokenInfo{Name: t.Name, Scope: t.Scope, CreatedAt: t.CreatedAt, LastSeen: t.LastSeen})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	httpjson.WriteJSON(w, http.StatusOK, list)
}

func (s *Server) tokensCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		UserEmail string `json:"user_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("name required"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpjson.WriteError(w, apierrors.Invalid("name required"))
		return
	}
	email := strings.TrimSpace(req.UserEmail)
	if email == "" {
		email = identityFrom(r).Email
	}

	token, err := generateToken()
	if err != nil {
		s.Log.Error("tokens: generation failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	info := TokenInfo{
		Hash:      hashToken(token),
		Name:      name,
		Scope:     scopeSync,
		UserEmail: email,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.tokens[info.Hash] = info
	s.saveTokens()
	s.mu.Unlock()

	httpjson.WriteJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"name":       info.Name,
		"scope":      info.Scope,
		"created_at": info.CreatedAt,
	})
}

func (s *Server) tokensDelete(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	s.mu.Lock()
	for k, v := range s.tokens {
		if v.Name == name {
			delete(s.tokens, k)
		}
	}
	s.saveTokens()
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
