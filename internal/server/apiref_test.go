package server

import (
	"log/slog"
	"testing"

	docs "github.com/FacileStudio/Jardin/internal/documentation"
	"github.com/FacileStudio/tronc/apiref"
)

// TestEveryRouteIsDocumented is the drift guard: it walks the router the binary
// actually serves and fails when a route is missing from the registry, so the
// reference at /docs cannot quietly describe an API Jardin no longer has.
//
// SSO_ONLY removes the password login route, so the router is built with it off
// to cover the larger surface.
func TestEveryRouteIsDocumented(t *testing.T) {
	server := &Server{Log: slog.Default(), DataDir: t.TempDir()}

	missing := apiref.Undocumented(server.Handler(), docs.Reference())
	if len(missing) > 0 {
		t.Errorf("routes missing from the API registry: %v", missing)
	}
}
