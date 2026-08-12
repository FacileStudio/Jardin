package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/FacileStudio/Mycelium/internal/sessions"

	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// claimsList answers with every active claim, liveness resolved against now
// rather than stored: a machine that has gone quiet since the claim was
// written must stop advertising itself as working.
func (s *Server) claimsList(w http.ResponseWriter, r *http.Request) {
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	entries := sessions.ReadClaimsLive(root, "", time.Now())
	if entries == nil {
		entries = []sessions.ClaimEntry{}
	}
	httpjson.WriteJSON(w, http.StatusOK, entries)
}

// claimsRelease drops a single (project, machine, agent) claim file. Deleting
// an already-absent claim is not an error: a second release request racing
// the first must not surface as a failure.
func (s *Server) claimsRelease(w http.ResponseWriter, r *http.Request) {
	project := pathParam(r, "project")
	machine := pathParam(r, "machine")
	agent := pathParam(r, "agent")
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	if err := sessions.ReleaseClaim(root, project, machine, agent); err != nil {
		s.Log.Error("claims: release failed", slog.Any("error", err))
		httpjson.WriteError(w, apierrors.Internal("internal error", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
