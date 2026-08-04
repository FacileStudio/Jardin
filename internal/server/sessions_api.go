package server

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/FacileStudio/Jardin/internal/sessions"
)

type sessionsStatsResponse struct {
	By   string             `json:"by"`
	Rows []sessions.StatRow `json:"rows"`
}

func (s *Server) sessionsStats(w http.ResponseWriter, r *http.Request) {
	since, err := sessions.ParseSince(r.URL.Query().Get("since"), time.Now())
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
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
	blocks, err := sessions.ReadBlocks(s.DataDir)
	if err != nil {
		log.Printf("sessions: read failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	rows := sessions.Aggregate(blocks, since, by)
	if rows == nil {
		rows = []sessions.StatRow{}
	}
	jsonReply(w, sessionsStatsResponse{By: by, Rows: rows})
}

func (s *Server) sessionsRecent(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	blocks, err := sessions.ReadBlocks(s.DataDir)
	if err != nil {
		log.Printf("sessions: read failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	recent := sessions.Recent(blocks, limit)
	if recent == nil {
		recent = []sessions.Block{}
	}
	jsonReply(w, recent)
}
