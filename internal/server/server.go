package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FacileStudio/Ruche/internal/memory"
)

const (
	scopeAdmin = "admin"
	scopeSync  = "sync"

	loginMaxAttempts = 10
	loginWindow      = time.Minute
)

type Server struct {
	DataDir   string
	Password  string
	mu        sync.RWMutex
	tokens    map[string]TokenInfo
	logins    *rateLimiter
	devices   *deviceStore
	devStarts *rateLimiter
	devPolls  *rateLimiter
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
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
}

type StatusResponse struct {
	Machine string   `json:"machine"`
	Rules   []string `json:"rules"`
	Skills  []string `json:"skills"`
}

func New(dataDir, password string) *Server {
	s := &Server{
		DataDir:   dataDir,
		Password:  password,
		tokens:    make(map[string]TokenInfo),
		logins:    newRateLimiter(loginMaxAttempts, loginWindow),
		devices:   newDeviceStore(),
		devStarts: newRateLimiter(20, time.Minute),
		devPolls:  newRateLimiter(120, time.Minute),
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
		log.Printf("tokens: failed to parse %s: %v", s.tokensPath(), err)
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
		log.Printf("tokens: marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		log.Printf("tokens: mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(s.tokensPath(), data, 0o600); err != nil {
		log.Printf("tokens: write failed: %v", err)
	}
}

func (s *Server) memoryDir() string { return filepath.Join(s.DataDir, "memory") }
func (s *Server) rulesDir() string  { return filepath.Join(s.DataDir, "rules") }
func (s *Server) skillsDir() string { return filepath.Join(s.DataDir, "skills") }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/config", s.authConfig)
	mux.HandleFunc("POST /api/auth/login", s.login)

	mux.HandleFunc("POST /api/auth/device/start", s.deviceStart)
	mux.HandleFunc("POST /api/auth/device/poll", s.devicePoll)
	mux.HandleFunc("GET /api/auth/device/info", s.auth(true, s.deviceInfo))
	mux.HandleFunc("POST /api/auth/device/approve", s.auth(true, s.deviceApprove))
	mux.HandleFunc("POST /api/auth/device/deny", s.auth(true, s.deviceDeny))

	mux.HandleFunc("GET /api/status", s.auth(false, s.status))

	mux.HandleFunc("GET /api/memory/search", s.auth(false, s.memorySearch))
	mux.HandleFunc("GET /api/memory/index", s.auth(false, s.memoryIndex))

	mux.HandleFunc("GET /api/rules", s.auth(false, s.rulesList))
	mux.HandleFunc("GET /api/rules/{name}", s.auth(false, s.ruleGet))
	mux.HandleFunc("PUT /api/rules/{name}", s.auth(false, s.ruleSave))
	mux.HandleFunc("DELETE /api/rules/{name}", s.auth(false, s.ruleDelete))

	mux.HandleFunc("GET /api/skills", s.auth(false, s.skillsList))
	mux.HandleFunc("GET /api/skills/{name}", s.auth(false, s.skillGet))
	mux.HandleFunc("PUT /api/skills/{name}", s.auth(false, s.skillSave))
	mux.HandleFunc("DELETE /api/skills/{name}", s.auth(false, s.skillDelete))

	mux.HandleFunc("GET /api/tokens", s.auth(true, s.tokensList))
	mux.HandleFunc("POST /api/tokens", s.auth(true, s.tokensCreate))
	mux.HandleFunc("DELETE /api/tokens/{name}", s.auth(true, s.tokensDelete))

	mux.HandleFunc("GET /api/sync/tree", s.auth(false, s.syncTree))
	mux.HandleFunc("GET /api/sync/files/{path...}", s.auth(false, s.syncGetFile))
	mux.HandleFunc("PUT /api/sync/files/{path...}", s.auth(false, s.syncPutFile))
	mux.HandleFunc("DELETE /api/sync/files/{path...}", s.auth(false, s.syncDeleteFile))

	return mux
}

func (s *Server) auth(adminOnly bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Password == "" {
			next(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		hash := hashToken(strings.TrimPrefix(header, "Bearer "))

		s.mu.Lock()
		info, ok := s.tokens[hash]
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
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if adminOnly && info.Scope != scopeAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, map[string]bool{
		"sso_only":     os.Getenv("SSO_ONLY") == "true",
		"oidc_enabled": os.Getenv("OIDC_ENABLED") == "true",
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.logins.allow(clientIP(r), time.Now()) {
		http.Error(w, "too many attempts", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Password string `json:"password"`
		Machine  string `json:"machine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.Password)) != 1 {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(req.Machine)
	scope := scopeSync
	if name == "" {
		name = "session"
		scope = scopeAdmin
	}

	token, err := s.mintToken(name, scope)
	if err != nil {
		log.Printf("login: token generation failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonReply(w, map[string]string{"token": token})
}

// mintToken generates a new bearer token, stores its hash under the given
// machine name (replacing any prior token with the same name), and returns the
// plaintext token to hand to the caller exactly once.
func (s *Server) mintToken(name, scope string) (string, error) {
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
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.saveTokens()
	s.mu.Unlock()
	return token, nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		Rules:  listMdNames(s.rulesDir()),
		Skills: listMdNames(s.skillsDir()),
	}
	jsonReply(w, resp)
}

func (s *Server) memorySearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonReply(w, []memory.SearchResult{})
		return
	}
	results, err := memory.Search(s.memoryDir(), query)
	if err != nil {
		log.Printf("memory search: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []memory.SearchResult{}
	}
	jsonReply(w, results)
}

func (s *Server) memoryIndex(w http.ResponseWriter, r *http.Request) {
	content, err := memory.ReadIndex(s.memoryDir())
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, listMdNames(s.rulesDir()))
}

func (s *Server) ruleGet(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.rulesDir(), name+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) ruleSave(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := writeFile(s.rulesDir(), name+".md", r.Body); err != nil {
		log.Printf("rule save %q: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	os.Remove(filepath.Join(s.rulesDir(), name+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillsList(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, listMdNames(s.skillsDir()))
}

func (s *Server) skillGet(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.skillsDir(), name+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) skillSave(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := writeFile(s.skillsDir(), name+".md", r.Body); err != nil {
		log.Printf("skill save %q: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillDelete(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(r.PathValue("name"))
	if !ok {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	os.Remove(filepath.Join(s.skillsDir(), name+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncTree(w http.ResponseWriter, r *http.Request) {
	var files []FileEntry
	filepath.Walk(s.DataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.DataDir, path)
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
	jsonReply(w, files)
}

func (s *Server) syncGetFile(w http.ResponseWriter, r *http.Request) {
	full, ok := s.resolveSyncPath(r.PathValue("path"))
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, full)
}

func (s *Server) syncPutFile(w http.ResponseWriter, r *http.Request) {
	full, ok := s.resolveSyncPath(r.PathValue("path"))
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		log.Printf("sync put: mkdir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		log.Printf("sync put: write: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncDeleteFile(w http.ResponseWriter, r *http.Request) {
	full, ok := s.resolveSyncPath(r.PathValue("path"))
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		log.Printf("sync delete: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
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
	jsonReply(w, list)
}

func (s *Server) tokensCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	token, err := generateToken()
	if err != nil {
		log.Printf("tokens: generation failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	info := TokenInfo{
		Hash:      hashToken(token),
		Name:      name,
		Scope:     scopeSync,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.tokens[info.Hash] = info
	s.saveTokens()
	s.mu.Unlock()

	jsonReply(w, map[string]string{
		"token":      token,
		"name":       info.Name,
		"scope":      info.Scope,
		"created_at": info.CreatedAt,
	})
}

func (s *Server) tokensDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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

func (s *Server) resolveSyncPath(rel string) (string, bool) {
	clean := strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	if clean == "." || clean == "tokens.json" || strings.HasPrefix(clean, ".") || strings.HasSuffix(clean, ".conflict") {
		return "", false
	}
	full := filepath.Join(s.DataDir, clean)
	if full != s.DataDir && !strings.HasPrefix(full, s.DataDir+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}

func syncSkip(rel string) bool {
	return rel == "tokens.json" || strings.HasPrefix(rel, ".") || strings.HasSuffix(rel, ".conflict")
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

func jsonReply(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
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
