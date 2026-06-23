package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FacileStudio/Ruche/internal/memory"
)

type Server struct {
	DataDir  string
	Password string
	mu       sync.RWMutex
	tokens   map[string]TokenInfo
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

type TokenInfo struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type StatusResponse struct {
	Machine string   `json:"machine"`
	Rules   []string `json:"rules"`
	Skills  []string `json:"skills"`
}

func New(dataDir, password string) *Server {
	s := &Server{
		DataDir:  dataDir,
		Password: password,
		tokens:   make(map[string]TokenInfo),
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
	var tokens map[string]TokenInfo
	if err := json.Unmarshal(data, &tokens); err == nil {
		s.tokens = tokens
	}
}

func (s *Server) saveTokens() {
	data, _ := json.MarshalIndent(s.tokens, "", "  ")
	os.MkdirAll(s.DataDir, 0755)
	os.WriteFile(s.tokensPath(), data, 0600)
}

func (s *Server) memoryDir() string    { return filepath.Join(s.DataDir, "memory") }
func (s *Server) rulesDir() string    { return filepath.Join(s.DataDir, "rules") }
func (s *Server) skillsDir() string   { return filepath.Join(s.DataDir, "skills") }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/config", s.authConfig)
	mux.HandleFunc("POST /api/auth/login", s.login)

	mux.HandleFunc("GET /api/status", s.auth(s.status))

	mux.HandleFunc("GET /api/memory/search", s.auth(s.memorySearch))
	mux.HandleFunc("GET /api/memory/index", s.auth(s.memoryIndex))

	mux.HandleFunc("GET /api/rules", s.auth(s.rulesList))
	mux.HandleFunc("GET /api/rules/{name}", s.auth(s.ruleGet))
	mux.HandleFunc("PUT /api/rules/{name}", s.auth(s.ruleSave))
	mux.HandleFunc("DELETE /api/rules/{name}", s.auth(s.ruleDelete))

	mux.HandleFunc("GET /api/skills", s.auth(s.skillsList))
	mux.HandleFunc("GET /api/skills/{name}", s.auth(s.skillGet))
	mux.HandleFunc("PUT /api/skills/{name}", s.auth(s.skillSave))
	mux.HandleFunc("DELETE /api/skills/{name}", s.auth(s.skillDelete))

	mux.HandleFunc("GET /api/tokens", s.auth(s.tokensList))
	mux.HandleFunc("POST /api/tokens", s.auth(s.tokensCreate))
	mux.HandleFunc("DELETE /api/tokens/{name}", s.auth(s.tokensDelete))

	mux.HandleFunc("GET /api/sync/tree", s.auth(s.syncTree))
	mux.HandleFunc("GET /api/sync/files/{path...}", s.auth(s.syncGetFile))
	mux.HandleFunc("PUT /api/sync/files/{path...}", s.auth(s.syncPutFile))

	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
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
		token := strings.TrimPrefix(header, "Bearer ")

		s.mu.RLock()
		_, validToken := s.tokens[token]
		s.mu.RUnlock()

		if !validToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	ssoOnly := os.Getenv("SSO_ONLY") == "true"
	oidcEnabled := os.Getenv("OIDC_ENABLED") == "true"
	jsonReply(w, map[string]bool{
		"sso_only":     ssoOnly,
		"oidc_enabled": oidcEnabled,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Machine  string `json:"machine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Password != s.Password {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(req.Machine)
	if name == "" {
		name = "session"
	}

	token := generateToken()
	s.mu.Lock()
	for k, v := range s.tokens {
		if v.Name == name {
			delete(s.tokens, k)
		}
	}
	s.tokens[token] = TokenInfo{
		Token:     token,
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.saveTokens()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, listMdNames(s.rulesDir()))
}

func (s *Server) ruleGet(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(s.rulesDir(), r.PathValue("name")+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) ruleSave(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	dir := s.rulesDir()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, r.PathValue("name")+".md"), data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	os.Remove(filepath.Join(s.rulesDir(), r.PathValue("name")+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillsList(w http.ResponseWriter, r *http.Request) {
	jsonReply(w, listMdNames(s.skillsDir()))
}

func (s *Server) skillGet(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(s.skillsDir(), r.PathValue("name")+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) skillSave(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	dir := s.skillsDir()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, r.PathValue("name")+".md"), data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillDelete(w http.ResponseWriter, r *http.Request) {
	os.Remove(filepath.Join(s.skillsDir(), r.PathValue("name")+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncTree(w http.ResponseWriter, r *http.Request) {
	var files []FileEntry
	filepath.Walk(s.DataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.DataDir, path)
		if rel == "tokens.json" || strings.HasPrefix(rel, ".") {
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
	filePath := r.PathValue("path")
	fullPath := filepath.Join(s.DataDir, filePath)
	if !strings.HasPrefix(fullPath, s.DataDir) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, fullPath)
}

func (s *Server) syncPutFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.PathValue("path")
	fullPath := filepath.Join(s.DataDir, filePath)
	if !strings.HasPrefix(fullPath, s.DataDir) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	data, _ := io.ReadAll(r.Body)
	os.WriteFile(fullPath, data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tokensList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []TokenInfo
	for _, t := range s.tokens {
		list = append(list, TokenInfo{Name: t.Name, CreatedAt: t.CreatedAt})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	jsonReply(w, list)
}

func (s *Server) tokensCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	token := generateToken()
	info := TokenInfo{
		Token:     token,
		Name:      req.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.tokens[token] = info
	s.saveTokens()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
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

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
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
