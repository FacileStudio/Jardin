package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

const (
	scopeAdmin = "admin"
	scopeSync  = "sync"
	scopeUser  = "user"

	loginMaxAttempts = 10
	loginWindow      = time.Minute
)

type ctxKey int

const identityKey ctxKey = 0

// Identity is the authenticated caller as the handlers see it.
type Identity struct {
	Email     string
	Scope     string
	TokenHash string
}

// identityFrom pulls the authenticated Identity out of a request context.
func identityFrom(r *http.Request) Identity {
	id, _ := r.Context().Value(identityKey).(Identity)
	return id
}

// auth guards every authenticated handler. A session token's scope is frozen
// at mint time, but admin status can change afterward (promotion, demotion,
// .users.json edits), so for any token bound to a user email the scope is
// re-derived from the live user record instead of trusting what was baked in
// at login.
func (s *Server) auth(adminOnly bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Password == "" && s.OIDC == nil {
			ctx := context.WithValue(r.Context(), identityKey, Identity{Scope: scopeAdmin})
			next(w, r.WithContext(ctx))
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
			return
		}
		hash := hashToken(strings.TrimPrefix(header, "Bearer "))
		info, ok := s.touchToken(hash)
		if !ok {
			httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
			return
		}
		scope := s.effectiveScope(info)
		if adminOnly && scope != scopeAdmin {
			httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
			return
		}
		ctx := context.WithValue(r.Context(), identityKey, Identity{
			Email: info.UserEmail, Scope: scope, TokenHash: hash,
		})
		next(w, r.WithContext(ctx))
	}
}

// touchToken resolves a token hash, evicting it first if it has expired, and
// records that it was just seen. The last-seen write is throttled to a minute
// so a busy client does not rewrite the token file on every request.
func (s *Server) touchToken(hash string) (TokenInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.tokens[hash]
	if !ok {
		return TokenInfo{}, false
	}
	if info.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil && time.Now().UTC().After(exp) {
			delete(s.tokens, hash)
			s.saveTokens()
			return TokenInfo{}, false
		}
	}
	now := time.Now().UTC()
	prev, _ := time.Parse(time.RFC3339, info.LastSeen)
	info.LastSeen = now.Format(time.RFC3339)
	s.tokens[hash] = info
	if now.Sub(prev) > time.Minute {
		s.saveTokens()
	}
	return info, true
}

// effectiveScope re-derives a token's scope from the live user record. A
// session's scope is frozen at mint time while admin status is not, so a
// promotion or demotion takes effect on the next request rather than at the
// next login, and a token minted as admin drops to user once its account is
// no longer one.
func (s *Server) effectiveScope(info TokenInfo) string {
	if info.UserEmail == "" {
		return info.Scope
	}
	s.mu.RLock()
	user, known := s.loadUsers()[info.UserEmail]
	s.mu.RUnlock()
	if known && user.Admin {
		return scopeAdmin
	}
	if info.Scope == scopeAdmin {
		return scopeUser
	}
	return info.Scope
}

// commonAllowed reports whether an identity may touch the common tree. The
// common tree is the instance owner's private data, not a shared bucket:
// admins reach it, machine tokens reach it (only an admin can mint one, and
// pre-multi-user tokens carry no email), and every other signed-in user is
// denied until an admin adds them to a space.
func (s *Server) commonAllowed(id Identity) bool {
	if id.Scope == scopeAdmin {
		return true
	}
	if id.Scope == scopeSync {
		if id.Email == "" {
			return true
		}
		s.mu.RLock()
		user, ok := s.loadUsers()[id.Email]
		s.mu.RUnlock()
		return ok && user.Admin
	}
	return false
}

// scopeRoot resolves the tree a request operates on: the common tree when no
// space_id is supplied, or the space's subtree after the membership guard
// passes. Writes its own error response on failure.
func (s *Server) scopeRoot(w http.ResponseWriter, r *http.Request) (string, bool) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		if !s.commonAllowed(identityFrom(r)) {
			httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
			return "", false
		}
		return s.DataDir, true
	}
	if strings.ContainsAny(spaceID, "/\\.") {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return "", false
	}
	if _, _, ok := s.spaceAccess(identityFrom(r), spaceID); !ok {
		httpjson.WriteError(w, apierrors.Forbidden("forbidden"))
		return "", false
	}
	return s.spaceDir(spaceID), true
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{
		"password_auth":  s.Password != "" && !s.SSOOnly,
		"sso_only":       s.SSOOnly,
		"oidc_enabled":   s.OIDC != nil,
		"device_enabled": true,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.limits.logins.allow(clientIP(r), time.Now()) {
		httpjson.WriteError(w, apierrors.RateLimited("too many attempts"))
		return
	}

	var req struct {
		Password string `json:"password"`
		Machine  string `json:"machine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.Password)) != 1 {
		httpjson.WriteError(w, apierrors.Unauthorized("invalid password"))
		return
	}

	token, err := s.mintLogin(strings.TrimSpace(req.Machine))
	if err != nil {
		s.Log.Error("login: token generation failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}

	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
}

// mintLogin issues what a password login is asking for: a machine token when
// the caller named a machine, and a browser session when it did not.
//
// The browser half goes through mintNamedSession so it carries an expiry and
// evicts the session it replaces, exactly as every OIDC session already does.
// That is what makes "a login session is the entry with an expiry" true rather
// than nearly true. Until it did, this path minted an admin credential that
// never expired, and the tokens page listed it as an API token with a revoke
// button beside it — the half of the 2026-08-25 fix that was missed, because
// the names it looked for were the two the OIDC path writes.
func (s *Server) mintLogin(machine string) (string, error) {
	if machine != "" {
		return s.mintToken(machine, scopeSync, "")
	}
	return s.mintNamedSession(passwordSessionName, "", scopeAdmin)
}

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string][]time.Time),
		max:      max,
		window:   window,
	}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := now.Add(-rl.window)
	recent := rl.attempts[key][:0]
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= rl.max {
		rl.attempts[key] = recent
		return false
	}
	rl.attempts[key] = append(recent, now)
	return true
}
