package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	docs "github.com/FacileStudio/Mycelium/internal/documentation"
	"github.com/FacileStudio/Mycelium/internal/env"
	apierrors "github.com/FacileStudio/tronc/errors"

	"github.com/FacileStudio/tronc/apiref"
	"github.com/FacileStudio/tronc/health"
	"github.com/FacileStudio/tronc/httpjson"
	"github.com/FacileStudio/tronc/httpx"
	"github.com/FacileStudio/tronc/middleware"
)

// serverLimits holds the per-IP rate limiters. They are grouped because they
// are one concern with three thresholds: a password login is cheap to guess,
// a device grant is not, and a device poll is expected to repeat.
type serverLimits struct {
	logins    *rateLimiter
	devStarts *rateLimiter
	devPolls  *rateLimiter
}

// Server is the HTTP API. Tokens, rate limiters and the emitter live here so
// handlers share one stateful process.
type Server struct {
	DataDir            string
	Password           string
	SSOOnly            bool
	OIDC               *env.OIDC
	CORSAllowedOrigins []string
	Log                *slog.Logger
	mu                 sync.RWMutex
	tokens             map[string]TokenInfo
	devices            *deviceStore
	loginCodes         *loginCodeStore
	limits             serverLimits
	emitter            *Emitter
	oidc               oidcRuntime

	// Semantic is the vector half of memory search. Nil leaves it dormant:
	// the server starts, search answers lexically, and nothing enqueues.
	Semantic *Semantic
}

// New builds a Server over a data directory with the given bootstrap
// password.
func New(dataDir, password string) *Server {
	s := &Server{
		DataDir:  dataDir,
		Password: password,
		Log:      slog.Default(),
		tokens:   make(map[string]TokenInfo),
		limits: serverLimits{
			logins:    newRateLimiter(loginMaxAttempts, loginWindow),
			devStarts: newRateLimiter(20, time.Minute),
			devPolls:  newRateLimiter(120, time.Minute),
		},
		devices:    newDeviceStore(),
		loginCodes: newLoginCodeStore(),
	}
	s.loadTokens()
	return s
}

// Handler builds the HTTP router: the suite's standard middleware stack, the
// liveness and readiness probes at both the root and /api, and every API route
// under a single /api subtree so an unknown API path answers 404 instead of
// falling through to the SPA catch-all the caller mounts on the returned mux.
func (s *Server) Handler() *chi.Mux {
	router := httpx.NewRouter(httpx.Config{
		Logger: s.Log,
		CORS:   middleware.CORSConfig{AllowedOrigins: s.CORSAllowedOrigins},
	})
	health.Mount(router, s.dataDirCheck())
	apiref.Mount(router, docs.Reference())

	router.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpjson.WriteError(w, apierrors.NotFound("no such endpoint"))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeStatusError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})

		s.mountAuthRoutes(r)
		s.mountInsightRoutes(r)
		s.mountContentRoutes(r)
		s.mountAccountRoutes(r)
		s.mountSyncRoutes(r)
	})

	return router
}

// The mount helpers below are called in the order their routes were declared
// in, and each keeps its own group intact. Route paths and their order are
// checked against internal/documentation's registry by a drift test, so a
// route moving between groups is caught rather than merely reviewed.

func (s *Server) mountAuthRoutes(r chi.Router) {
	r.Get("/auth/config", s.authConfig)
	if !s.SSOOnly {
		r.Post("/auth/login", s.login)
	}
	r.Get("/auth/oidc", s.oidcStart)
	r.Get("/auth/oidc/callback", s.oidcCallback)
	r.Post("/auth/oidc/exchange", s.oidcExchange)
	r.Get("/auth/me", s.auth(false, s.authMe))
	r.Post("/auth/logout", s.auth(false, s.logout))

	r.Post("/auth/device/start", s.deviceStart)
	r.Post("/auth/device/poll", s.devicePoll)
	r.Get("/auth/device/info", s.auth(true, s.deviceInfo))
	r.Post("/auth/device/approve", s.auth(true, s.deviceApprove))
	r.Post("/auth/device/deny", s.auth(true, s.deviceDeny))
}

func (s *Server) mountInsightRoutes(r chi.Router) {
	r.Get("/status", s.auth(false, s.status))

	r.Get("/memory/search", s.auth(false, s.memorySearch))
	r.Post("/memory/search", s.auth(false, s.memorySearchPost))
	r.Get("/memory/index", s.auth(false, s.memoryIndex))
	r.Get("/memory/index/status", s.auth(false, s.memoryIndexStatus))

	r.Get("/sessions/stats", s.auth(false, s.sessionsStats))
	r.Get("/sessions/recent", s.auth(false, s.sessionsRecent))
	r.Get("/sessions/live", s.auth(false, s.sessionsLive))
	r.Get("/sessions/timeline", s.auth(false, s.sessionsTimeline))

	r.Get("/claims", s.auth(false, s.claimsList))
	r.Delete("/claims/{project}/{machine}/{agent}", s.auth(false, s.claimsRelease))

	r.Get("/usage", s.auth(false, s.usageCurrent))
	r.Get("/usage/history", s.auth(false, s.usageHistory))
}

func (s *Server) mountContentRoutes(r chi.Router) {
	r.Get("/settings", s.auth(true, s.settingsGet))
	r.Put("/settings", s.auth(true, s.settingsPut))

	r.Get("/rules", s.auth(false, s.rulesList))
	r.Get("/rules/{name}", s.auth(false, s.ruleGet))
	r.Put("/rules/{name}", s.auth(false, s.ruleSave))
	r.Delete("/rules/{name}", s.auth(false, s.ruleDelete))

	r.Get("/skills", s.auth(false, s.skillsList))
	r.Get("/skills/{name}", s.auth(false, s.skillGet))
	r.Put("/skills/{name}", s.auth(false, s.skillSave))
	r.Delete("/skills/{name}", s.auth(false, s.skillDelete))

	r.Get("/flows", s.auth(false, s.flowsList))
	r.Get("/flows/{name}", s.auth(false, s.flowGet))

	r.Get("/models", s.auth(false, s.modelsList))
	r.Get("/models/*", s.auth(false, s.modelGet))
}

func (s *Server) mountAccountRoutes(r chi.Router) {
	r.Get("/users", s.auth(false, s.usersList))

	r.Get("/spaces", s.auth(false, s.spacesList))
	r.Post("/spaces", s.auth(false, s.spacesCreate))
	r.Put("/spaces/{id}", s.auth(false, s.spacesUpdate))
	r.Delete("/spaces/{id}", s.auth(false, s.spacesDelete))
	r.Get("/spaces/{id}/members", s.auth(false, s.spacesMembers))
	r.Post("/spaces/{id}/members", s.auth(false, s.spacesMemberAdd))
	r.Put("/spaces/{id}/members/{email}", s.auth(false, s.spacesMemberUpdate))
	r.Delete("/spaces/{id}/members/{email}", s.auth(false, s.spacesMemberRemove))
	r.Post("/spaces/{id}/leave", s.auth(false, s.spacesLeave))

	r.Get("/tokens", s.auth(true, s.tokensList))
	r.Post("/tokens", s.auth(true, s.tokensCreate))
	r.Delete("/tokens/{name}", s.auth(true, s.tokensDelete))
}

func (s *Server) mountSyncRoutes(r chi.Router) {
	r.Get("/sync/tree", s.auth(false, s.syncTree))
	r.Get("/sync/files/*", s.auth(false, s.syncGetFile))
	r.Put("/sync/files/*", s.auth(false, s.syncPutFile))
	r.Delete("/sync/files/*", s.auth(false, s.syncDeleteFile))
}

// dataDirCheck is the one dependency Mycelium has: a writable data directory. A
// named volume owned by root under a non-root process fails here rather than
// at the first write.
func (s *Server) dataDirCheck() health.Check {
	return func(context.Context) error {
		return os.MkdirAll(s.DataDir, 0o755)
	}
}
