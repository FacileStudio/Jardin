package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	docs "github.com/FacileStudio/Mycelium/internal/documentation"
	"github.com/FacileStudio/Mycelium/internal/env"
	"github.com/FacileStudio/Mycelium/internal/memory"
	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/apiref"
	"github.com/FacileStudio/tronc/health"
	"github.com/FacileStudio/tronc/httpjson"
	"github.com/FacileStudio/tronc/httpx"
	"github.com/FacileStudio/tronc/middleware"
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

type Identity struct {
	Email     string
	Scope     string
	TokenHash string
}

func identityFrom(r *http.Request) Identity {
	id, _ := r.Context().Value(identityKey).(Identity)
	return id
}

type Server struct {
	DataDir            string
	Password           string
	SSOOnly            bool
	OIDC               *env.OIDC
	CORSAllowedOrigins []string
	Log                *slog.Logger
	mu                 sync.RWMutex
	tokens             map[string]TokenInfo
	logins             *rateLimiter
	devices            *deviceStore
	loginCodes         *loginCodeStore
	devStarts          *rateLimiter
	devPolls           *rateLimiter
	emitter            *Emitter
	oidc               oidcRuntime
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

type TokenInfo struct {
	Hash      string `json:"-"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	UserEmail string `json:"user_email,omitempty"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	LastSeen  string `json:"last_seen"`
}

type StatusResponse struct {
	Machine string   `json:"machine"`
	Rules   []string `json:"rules"`
	Skills  []string `json:"skills"`
}

func New(dataDir, password string) *Server {
	s := &Server{
		DataDir:    dataDir,
		Password:   password,
		Log:        slog.Default(),
		tokens:     make(map[string]TokenInfo),
		logins:     newRateLimiter(loginMaxAttempts, loginWindow),
		devices:    newDeviceStore(),
		loginCodes: newLoginCodeStore(),
		devStarts:  newRateLimiter(20, time.Minute),
		devPolls:   newRateLimiter(120, time.Minute),
	}
	s.loadTokens()
	return s
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

// Handler builds the HTTP router: the suite's standard middleware stack, the
// liveness and readiness probes at both the root and /api, and every API route
// under a single /api subtree so an unknown API path answers 404 instead of
// falling through to the SPA catch-all the caller mounts on the returned mux.
func (s *Server) Handler() *chi.Mux {
	router := httpx.NewRouter(httpx.Config{
		Logger: s.Log,
		CORS:   middleware.CORSConfig{AllowedOrigins: s.CORSAllowedOrigins},
	})
	health.Mount(router, s.dataDirCheck())
	apiref.Mount(router, docs.Reference())

	router.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpjson.WriteError(w, apierrors.NotFound("no such endpoint"))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeStatusError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})

		r.Get("/auth/config", s.authConfig)
		if !s.SSOOnly {
			r.Post("/auth/login", s.login)
		}
		r.Get("/auth/oidc", s.oidcStart)
		r.Get("/auth/oidc/callback", s.oidcCallback)
		r.Post("/auth/oidc/exchange", s.oidcExchange)
		r.Get("/auth/me", s.auth(false, s.authMe))
		r.Post("/auth/logout", s.auth(false, s.logout))

		r.Post("/auth/device/start", s.deviceStart)
		r.Post("/auth/device/poll", s.devicePoll)
		r.Get("/auth/device/info", s.auth(true, s.deviceInfo))
		r.Post("/auth/device/approve", s.auth(true, s.deviceApprove))
		r.Post("/auth/device/deny", s.auth(true, s.deviceDeny))

		r.Get("/status", s.auth(false, s.status))

		r.Get("/memory/search", s.auth(false, s.memorySearch))
		r.Get("/memory/index", s.auth(false, s.memoryIndex))

		r.Get("/sessions/stats", s.auth(false, s.sessionsStats))
		r.Get("/sessions/recent", s.auth(false, s.sessionsRecent))
		r.Get("/sessions/live", s.auth(false, s.sessionsLive))
		r.Get("/sessions/timeline", s.auth(false, s.sessionsTimeline))

		r.Get("/claims", s.auth(false, s.claimsList))
		r.Delete("/claims/{project}/{machine}/{agent}", s.auth(false, s.claimsRelease))

		r.Get("/usage", s.auth(false, s.usageCurrent))
		r.Get("/usage/history", s.auth(false, s.usageHistory))

		r.Get("/settings", s.auth(true, s.settingsGet))
		r.Put("/settings", s.auth(true, s.settingsPut))

		r.Get("/rules", s.auth(false, s.rulesList))
		r.Get("/rules/{name}", s.auth(false, s.ruleGet))
		r.Put("/rules/{name}", s.auth(false, s.ruleSave))
		r.Delete("/rules/{name}", s.auth(false, s.ruleDelete))

		r.Get("/skills", s.auth(false, s.skillsList))
		r.Get("/skills/{name}", s.auth(false, s.skillGet))
		r.Put("/skills/{name}", s.auth(false, s.skillSave))
		r.Delete("/skills/{name}", s.auth(false, s.skillDelete))

		r.Get("/users", s.auth(false, s.usersList))

		r.Get("/spaces", s.auth(false, s.spacesList))
		r.Post("/spaces", s.auth(false, s.spacesCreate))
		r.Put("/spaces/{id}", s.auth(false, s.spacesUpdate))
		r.Delete("/spaces/{id}", s.auth(false, s.spacesDelete))
		r.Get("/spaces/{id}/members", s.auth(false, s.spacesMembers))
		r.Post("/spaces/{id}/members", s.auth(false, s.spacesMemberAdd))
		r.Put("/spaces/{id}/members/{email}", s.auth(false, s.spacesMemberUpdate))
		r.Delete("/spaces/{id}/members/{email}", s.auth(false, s.spacesMemberRemove))
		r.Post("/spaces/{id}/leave", s.auth(false, s.spacesLeave))

		r.Get("/tokens", s.auth(true, s.tokensList))
		r.Post("/tokens", s.auth(true, s.tokensCreate))
		r.Delete("/tokens/{name}", s.auth(true, s.tokensDelete))

		r.Get("/sync/tree", s.auth(false, s.syncTree))
		r.Get("/sync/files/*", s.auth(false, s.syncGetFile))
		r.Put("/sync/files/*", s.auth(false, s.syncPutFile))
		r.Delete("/sync/files/*", s.auth(false, s.syncDeleteFile))
	})

	return router
}

// dataDirCheck is the one dependency Mycelium has: a writable data directory. A
// named volume owned by root under a non-root process fails here rather than
// at the first write.
func (s *Server) dataDirCheck() health.Check {
	return func(context.Context) error {
		return os.MkdirAll(s.DataDir, 0o755)
	}
}

// writeStatusError answers with the suite's error envelope at a status
// tronc's code-to-status map does not cover, so 405 and 503 stay themselves
// instead of collapsing into the generic 500 WriteError would produce.
func writeStatusError(w http.ResponseWriter, status int, code, message string) {
	httpjson.WriteJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// pathParam reads a chi URL parameter and unescapes it.
//
// chi matches on the raw path whenever the request carries any percent-encoding
// and hands the parameter back still encoded, where net/http's ServeMux — which
// this router replaced — decoded it. Without this, `%2F` would sail past the
// traversal guards and an encodeURIComponent'd member email would never match a
// stored one.
func pathParam(r *http.Request, key string) string {
	raw := chi.URLParam(r, key)
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}

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

		s.mu.Lock()
		info, ok := s.tokens[hash]
		if ok && info.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil && time.Now().UTC().After(exp) {
				delete(s.tokens, hash)
				s.saveTokens()
				ok = false
			}
		}
		if ok {
			now := time.Now().UTC()
			prev, _ := time.Parse(time.RFC3339, info.LastSeen)
			info.LastSeen = now.Format(time.RFC3339)
			s.tokens[hash] = info
			if now.Sub(prev) > time.Minute {
				s.saveTokens()
			}
		}
		s.mu.Unlock()

		if !ok {
			httpjson.WriteError(w, apierrors.Unauthorized("unauthorized"))
			return
		}
		// A session token's scope is frozen at mint time, but admin status can
		// change afterward (promotion, demotion, .users.json edits). For any
		// token bound to a user email, re-derive the scope from the live user
		// record instead of trusting what was baked in at login.
		scope := info.Scope
		if info.UserEmail != "" {
			s.mu.RLock()
			user, known := s.loadUsers()[info.UserEmail]
			s.mu.RUnlock()
			if known && user.Admin {
				scope = scopeAdmin
			} else if scope == scopeAdmin {
				scope = scopeUser
			}
		}
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
	if !s.logins.allow(clientIP(r), time.Now()) {
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

	name := strings.TrimSpace(req.Machine)
	scope := scopeSync
	if name == "" {
		name = "session"
		scope = scopeAdmin
	}

	token, err := s.mintToken(name, scope, "")
	if err != nil {
		s.Log.Error("login: token generation failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}

	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
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

func (s *Server) syncTree(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	var files []FileEntry
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if syncSkip(rel) {
			return nil
		}
		data, _ := os.ReadFile(path)
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		files = append(files, FileEntry{
			Path:     rel,
			Checksum: checksum,
			Size:     info.Size(),
			ModTime:  info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if files == nil {
		files = []FileEntry{}
	}
	httpjson.WriteJSON(w, http.StatusOK, files)
}

func (s *Server) syncGetFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	http.ServeFile(w, r, full)
}

func (s *Server) syncPutFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		s.Log.Error("sync put: mkdir failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		s.Log.Error("sync put: write failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncDeleteFile(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	full, ok := s.resolveSyncPath(root, pathParam(r, "*"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid path"))
		return
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		s.Log.Error("sync delete failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tokensList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]TokenInfo, 0, len(s.tokens))
	for _, t := range s.tokens {
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

func (s *Server) resolveSyncPath(root, rel string) (string, bool) {
	clean := strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	if clean == "." || syncSkip(clean) {
		return "", false
	}
	full := filepath.Join(root, clean)
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

// syncSkip fences everything the file sync must never touch: server state
// (tokens, dotfiles), conflict backups, and the spaces subtree — space content
// is reachable only through its own scoped root, never via the common tree.
func syncSkip(rel string) bool {
	return rel == "tokens.json" ||
		strings.HasPrefix(rel, ".") ||
		strings.HasSuffix(rel, ".conflict") ||
		rel == "spaces" || strings.HasPrefix(rel, "spaces"+string(os.PathSeparator)) || strings.HasPrefix(rel, "spaces/")
}

func safeName(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", false
	}
	return name, true
}

func writeFile(dir, name string, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func listMdNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(names)
	return names
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
