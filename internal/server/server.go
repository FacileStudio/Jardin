package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	DataDir string
	Token   string
}

type FileEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

func New(dataDir, token string) *Server {
	return &Server{DataDir: dataDir, Token: token}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cells", s.auth(s.listCells))
	mux.HandleFunc("GET /api/cells/{cell}/tree", s.auth(s.tree))
	mux.HandleFunc("GET /api/cells/{cell}/files/{path...}", s.auth(s.getFile))
	mux.HandleFunc("PUT /api/cells/{cell}/files/{path...}", s.auth(s.putFile))
	mux.HandleFunc("DELETE /api/cells/{cell}/files/{path...}", s.auth(s.deleteFile))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" {
			next(w, r)
			return
		}
		token := r.Header.Get("Authorization")
		if token != "Bearer "+s.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) listCells(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode([]string{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cells []string
	for _, e := range entries {
		if e.IsDir() {
			cells = append(cells, e.Name())
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cells)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	cellName := r.PathValue("cell")
	cellDir := filepath.Join(s.DataDir, cellName)

	var files []FileEntry
	err := filepath.Walk(cellDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(cellDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		files = append(files, FileEntry{
			Path:     rel,
			Checksum: checksum,
			Size:     info.Size(),
			ModTime:  info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []FileEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
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

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
