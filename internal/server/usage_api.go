package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/FacileStudio/Jardin/internal/sessions"
	"github.com/FacileStudio/Jardin/internal/usage"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// usageCurrent answers with one snapshot per machine, or an empty array when
// nothing has been recorded yet: the dashboard renders its own "no data" state
// and must not have to treat that as a failure. Freshness is resolved against
// the clock here rather than read off disk, so a window that has since rolled
// over is reported as expired instead of as a current percentage.
func (s *Server) usageCurrent(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	snapshots, err := usage.ReadCurrent(root)
	if err != nil {
		s.Log.Error("usage: read failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, usage.Resolve(snapshots, time.Now()))
}

func (s *Server) usageHistory(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("since")
	if raw == "" {
		raw = "7d"
	}
	since, err := sessions.ParseSince(raw, time.Now())
	if err != nil {
		httpjson.WriteError(w, apierrors.Invalid("bad request"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, usage.History(root, since, r.URL.Query().Get("machine")))
}
