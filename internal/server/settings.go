package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

type NookSettings struct {
	Enabled       bool              `json:"enabled"`
	Instance      string            `json:"instance"`
	Secret        string            `json:"secret"`
	UserEmail     string            `json:"user_email"`
	MachineEmails map[string]string `json:"machine_emails,omitempty"`
	EmitSince     string            `json:"emit_since,omitempty"`
}

type Settings struct {
	Nook NookSettings `json:"nook"`
}

func (n *NookSettings) EmailFor(machine string) string {
	if email, ok := n.MachineEmails[machine]; ok && email != "" {
		return email
	}
	return n.UserEmail
}

func (s *Server) settingsPath() string {
	return filepath.Join(s.DataDir, ".settings.json")
}

func (s *Server) loadSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var settings Settings
	data, err := os.ReadFile(s.settingsPath())
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		s.Log.Error("settings: failed to parse", slog.String("path", s.settingsPath()), slog.Any("error", err))
		return Settings{}
	}
	return settings
}

func (s *Server) saveSettings(settings Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.settingsPath(), data, 0o600)
}

type settingsResponse struct {
	Nook   NookSettings  `json:"nook"`
	Status EmitterStatus `json:"status"`
}

func (s *Server) settingsGet(w http.ResponseWriter, r *http.Request) {
	settings := s.loadSettings()
	resp := settingsResponse{Nook: settings.Nook}
	if settings.Nook.MachineEmails == nil {
		resp.Nook.MachineEmails = map[string]string{}
	}
	if s.emitter != nil {
		resp.Status = s.emitter.Status()
	}
	httpjson.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) settingsPut(w http.ResponseWriter, r *http.Request) {
	var incoming Settings
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	if err := validateNook(&incoming.Nook); err != nil {
		httpjson.WriteError(w, apierrors.Invalid(err.Error()))
		return
	}
	current := s.loadSettings()
	if incoming.Nook.Enabled && incoming.Nook.EmitSince == "" {
		if current.Nook.EmitSince != "" {
			incoming.Nook.EmitSince = current.Nook.EmitSince
		} else {
			incoming.Nook.EmitSince = time.Now().UTC().Format(time.RFC3339)
		}
	}
	if err := s.saveSettings(incoming); err != nil {
		s.Log.Error("settings: save failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if s.emitter != nil {
		s.emitter.Kick()
	}
	s.settingsGet(w, r)
}

func validateNook(n *NookSettings) error {
	if !n.Enabled {
		return nil
	}
	if n.Instance == "" || n.Secret == "" {
		return fmt.Errorf("instance and secret required when enabled")
	}
	u, err := url.Parse(n.Instance)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("instance must be an http(s) url")
	}
	if n.EmitSince != "" {
		if _, err := time.Parse(time.RFC3339, n.EmitSince); err != nil {
			return fmt.Errorf("emit_since must be RFC3339")
		}
	}
	return nil
}
