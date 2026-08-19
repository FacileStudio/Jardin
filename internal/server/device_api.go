package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

func (s *Server) deviceStart(w http.ResponseWriter, r *http.Request) {
	if !s.devStarts.allow(clientIP(r), time.Now()) {
		httpjson.WriteError(w, apierrors.RateLimited("too many requests"))
		return
	}

	var req struct {
		Machine string `json:"machine"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	machine := strings.TrimSpace(req.Machine)
	if machine == "" {
		machine = "device"
	}

	dr, err := s.devices.create(machine, clientIP(r), time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrTooManyDevices) {
			httpjson.WriteError(w, apierrors.RateLimited("too many pending authorizations"))
			return
		}
		s.Log.Error("device: create failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}

	base := s.baseURL(r)
	httpjson.WriteJSON(w, http.StatusOK, map[string]any{
		"device_code":               dr.DeviceCode,
		"user_code":                 dr.UserCode,
		"machine":                   dr.Machine,
		"verification_uri":          base + "/authorize",
		"verification_uri_complete": base + "/authorize?code=" + dr.UserCode,
		"interval":                  devicePollInterval,
		"expires_in":                int(deviceCodeTTL.Seconds()),
	})
}

func (s *Server) devicePoll(w http.ResponseWriter, r *http.Request) {
	if !s.devPolls.allow(clientIP(r), time.Now()) {
		httpjson.WriteError(w, apierrors.RateLimited("too many requests"))
		return
	}

	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}

	status, token, ok := s.devices.poll(req.DeviceCode)
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("unknown or expired device code"))
		return
	}
	switch status {
	case deviceApproved:
		httpjson.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
	case deviceDenied:
		httpjson.WriteError(w, apierrors.Forbidden("authorization denied"))
	default:
		httpjson.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
	}
}

func (s *Server) deviceInfo(w http.ResponseWriter, r *http.Request) {
	req, ok := s.devices.info(r.URL.Query().Get("code"))
	if !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{
		"user_code": req.UserCode,
		"machine":   req.Machine,
		"ip":        req.IP,
		"status":    string(req.Status),
	})
}

func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	req, ok := s.devices.info(body.UserCode)
	if !ok || req.Status != devicePending {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	token, err := s.mintToken(req.Machine, scopeSync, identityFrom(r).Email)
	if err != nil {
		s.Log.Error("device: token mint failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	if _, ok := s.devices.approve(body.UserCode, token); !ok {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"machine": req.Machine})
}

func (s *Server) deviceDeny(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserCode string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	if !s.devices.deny(body.UserCode) {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
