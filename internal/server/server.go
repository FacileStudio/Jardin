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

	"github.com/FacileStudio/Ruche/internal/brain"
	"github.com/FacileStudio/Ruche/internal/config"
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
	ActiveCell string           `json:"active_cell"`
	Machine    string           `json:"machine"`
	SyncURL    string           `json:"sync_url"`
	Cells      []config.CellRef `json:"cells"`
	Rules      []string         `json:"rules"`
	Skills     []string         `json:"skills"`
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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/config", s.authConfig)
	mux.HandleFunc("POST /api/auth/login", s.login)

	mux.HandleFunc("GET /api/status", s.auth(s.status))

	mux.HandleFunc("GET /api/cells", s.auth(s.listCells))
	mux.HandleFunc("POST /api/cells", s.auth(s.createCell))
	mux.HandleFunc("POST /api/cells/use", s.auth(s.useCell))
	mux.HandleFunc("GET /api/cells/{cell}/tree", s.auth(s.tree))
	mux.HandleFunc("GET /api/cells/{cell}/files/{path...}", s.auth(s.getFile))
	mux.HandleFunc("PUT /api/cells/{cell}/files/{path...}", s.auth(s.putFile))
	mux.HandleFunc("DELETE /api/cells/{cell}/files/{path...}", s.auth(s.deleteFile))

	mux.HandleFunc("GET /api/brain/search", s.auth(s.brainSearch))
	mux.HandleFunc("GET /api/brain/index", s.auth(s.brainIndex))

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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Password != s.Password {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	token := generateToken()
	s.mu.Lock()
	s.tokens[token] = TokenInfo{
		Token:     token,
		Name:      "session",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.saveTokens()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (s *Server) activeCellPath() (string, string, error) {
	cfg, err := config.LoadRucheConfig()
	if err != nil {
		return "", "", err
	}
	if cfg.ActiveCell == "" {
		return "", "", fmt.Errorf("no active cell")
	}
	path, err := cfg.ActiveCellPath()
	return cfg.ActiveCell, path, err
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRucheConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rules, skills []string
	if cellPath, err := cfg.ActiveCellPath(); err == nil {
		rules = listMdNames(filepath.Join(cellPath, "rules"))
		skills = listMdNames(filepath.Join(cellPath, "skills"))
	}

	resp := StatusResponse{
		ActiveCell: cfg.ActiveCell,
		Machine:    cfg.Machine,
		SyncURL:    cfg.SyncURL,
		Cells:      cfg.Cells,
		Rules:      rules,
		Skills:     skills,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) listCells(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadRucheConfig()
	if err != nil {
		jsonReply(w, []string{})
		return
	}
	var names []string
	for _, c := range cfg.Cells {
		names = append(names, c.Name)
	}
	jsonReply(w, names)
}

func (s *Server) createCell(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	cellPath := filepath.Join(config.CellsDir(), req.Name)
	dirs := []string{
		"brain", "brain/bugs", "brain/tools", "brain/projects",
		"brain/conventions", "brain/syntheses", "rules", "skills", "machines",
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(cellPath, d), 0755)
	}

	cellCfg := &config.CellConfig{Name: req.Name}
	config.SaveCellConfig(cellPath, cellCfg)

	os.WriteFile(filepath.Join(cellPath, "brain", "index.md"), []byte("# Brain Index\n"), 0644)
	os.WriteFile(filepath.Join(cellPath, "brain", "overview.md"), []byte("# Overview\n"), 0644)
	os.WriteFile(filepath.Join(cellPath, "brain", "log.md"), []byte("# Log\n\nAppend-only.\n"), 0644)

	cfg, _ := config.LoadRucheConfig()
	cfg.AddCell(req.Name, cellPath)
	if cfg.ActiveCell == "" {
		cfg.ActiveCell = req.Name
	}
	config.SaveRucheConfig(cfg)

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) useCell(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadRucheConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.FindCell(req.Name) == nil {
		http.Error(w, "cell not found", http.StatusNotFound)
		return
	}
	cfg.ActiveCell = req.Name
	config.SaveRucheConfig(cfg)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	cellName := r.PathValue("cell")
	cellDir := filepath.Join(s.DataDir, cellName)

	var files []FileEntry
	filepath.Walk(cellDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(cellDir, path)
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

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	cellName := r.PathValue("cell")
	filePath := r.PathValue("path")
	fullPath := filepath.Join(s.DataDir, cellName, filePath)
	if !strings.HasPrefix(fullPath, filepath.Join(s.DataDir, cellName)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, fullPath)
}

func (s *Server) putFile(w http.ResponseWriter, r *http.Request) {
	cellName := r.PathValue("cell")
	filePath := r.PathValue("path")
	fullPath := filepath.Join(s.DataDir, cellName, filePath)
	if !strings.HasPrefix(fullPath, filepath.Join(s.DataDir, cellName)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	data, _ := io.ReadAll(r.Body)
	os.WriteFile(fullPath, data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	cellName := r.PathValue("cell")
	filePath := r.PathValue("path")
	fullPath := filepath.Join(s.DataDir, cellName, filePath)
	if !strings.HasPrefix(fullPath, filepath.Join(s.DataDir, cellName)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	os.Remove(fullPath)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) brainSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonReply(w, []brain.SearchResult{})
		return
	}
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	results, err := brain.Search(filepath.Join(cellPath, "brain"), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []brain.SearchResult{}
	}
	jsonReply(w, results)
}

func (s *Server) brainIndex(w http.ResponseWriter, r *http.Request) {
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, err := brain.ReadIndex(filepath.Join(cellPath, "brain"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}

func (s *Server) rulesList(w http.ResponseWriter, r *http.Request) {
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		jsonReply(w, []string{})
		return
	}
	jsonReply(w, listMdNames(filepath.Join(cellPath, "rules")))
}

func (s *Server) ruleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(cellPath, "rules", name+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) ruleSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(r.Body)
	path := filepath.Join(cellPath, "rules", name+".md")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	os.Remove(filepath.Join(cellPath, "rules", name+".md"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillsList(w http.ResponseWriter, r *http.Request) {
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		jsonReply(w, []string{})
		return
	}
	jsonReply(w, listMdNames(filepath.Join(cellPath, "skills")))
}

func (s *Server) skillGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(cellPath, "skills", name+".md"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func (s *Server) skillSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(r.Body)
	path := filepath.Join(cellPath, "skills", name+".md")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) skillDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, cellPath, err := s.activeCellPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	os.Remove(filepath.Join(cellPath, "skills", name+".md"))
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
