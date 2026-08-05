package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/FacileStudio/Mycelium/internal/sessions"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

type sessionsStatsResponse struct {
	By   string             `json:"by"`
	Rows []sessions.StatRow `json:"rows"`
}

func (s *Server) sessionsStats(w http.ResponseWriter, r *http.Request) {
	since, err := sessions.ParseSince(r.URL.Query().Get("since"), time.Now())
	if err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	by := r.URL.Query().Get("by")
	valid := false
	for _, k := range sessions.GroupKeys {
		if k == by {
			valid = true
		}
	}
	if !valid {
		by = "project"
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	blocks, err := sessions.ReadBlocks(root)
	if err != nil {
		s.Log.Error("sessions: read failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	rows := sessions.Aggregate(blocks, since, by)
	if rows == nil {
		rows = []sessions.StatRow{}
	}
	httpjson.WriteJSON(w, http.StatusOK, sessionsStatsResponse{By: by, Rows: rows})
}

func (s *Server) sessionsLive(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	entries, err := sessions.ReadLive(root, time.Now())
	if err != nil {
		s.Log.Error("sessions: live read failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, entries)
}

func (s *Server) sessionsRecent(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	blocks, err := sessions.ReadBlocks(root)
	if err != nil {
		s.Log.Error("sessions: read failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	recent := sessions.Recent(blocks, limit)
	if recent == nil {
		recent = []sessions.Block{}
	}
	httpjson.WriteJSON(w, http.StatusOK, recent)
}
