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

type AntenneSettings struct {
	Enabled       bool              `json:"enabled"`
	Instance      string            `json:"instance"`
	Secret        string            `json:"secret"`
	UserEmail     string            `json:"user_email"`
	MachineEmails map[string]string `json:"machine_emails,omitempty"`
	EmitSince     string            `json:"emit_since,omitempty"`
}

type Settings struct {
	Antenne AntenneSettings  `json:"antenne"`
	Legacy  *AntenneSettings `json:"nook,omitempty"`
}

func (s *Settings) adoptLegacy() {
	if s.Legacy == nil {
		return
	}
	if s.Antenne.Instance == "" && !s.Antenne.Enabled {
		s.Antenne = *s.Legacy
	}
	s.Legacy = nil
}

func (n *AntenneSettings) EmailFor(machine string) string {
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
	settings.adoptLegacy()
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
	Antenne AntenneSettings `json:"antenne"`
	Status  EmitterStatus   `json:"status"`
}

func (s *Server) settingsGet(w http.ResponseWriter, r *http.Request) {
	settings := s.loadSettings()
	resp := settingsResponse{Antenne: settings.Antenne}
	if settings.Antenne.MachineEmails == nil {
		resp.Antenne.MachineEmails = map[string]string{}
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
	if err := validateAntenne(&incoming.Antenne); err != nil {
		httpjson.WriteError(w, apierrors.Invalid(err.Error()))
		return
	}
	current := s.loadSettings()
	if incoming.Antenne.Enabled && incoming.Antenne.EmitSince == "" {
		if current.Antenne.EmitSince != "" {
			incoming.Antenne.EmitSince = current.Antenne.EmitSince
		} else {
			incoming.Antenne.EmitSince = time.Now().UTC().Format(time.RFC3339)
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

func validateAntenne(n *AntenneSettings) error {
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
