package cmd

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/FacileStudio/Mycelium/internal/env"
	"github.com/FacileStudio/Mycelium/internal/memory"
	"github.com/FacileStudio/Mycelium/internal/server"
	"github.com/FacileStudio/Journal/sdk/journal"
	"github.com/FacileStudio/tronc/logger"
	"github.com/FacileStudio/tronc/spa"
	"github.com/spf13/cobra"
)

const (
	// semanticProbeTimeout is deliberately short. The probe runs before the
	// listener starts, so it is the one place an optional backend can hold the
	// whole server down: a sidecar that resolves but hangs would otherwise eat
	// most of the healthcheck's start window and invite a restart loop. A
	// sidecar on the same host answers in well under this.
	semanticProbeTimeout = 3 * time.Second
	vectorIndexDir       = ".embeddings"
	qdrantCollection     = "mycelium_memory"
)

var servePort int

// attachSemantic wires the vector half of memory search onto the server. A
// failure here is logged and swallowed: search must fall back to lexical rather
// than keep the server from starting, which is the same bargain every caller of
// the search endpoint makes.
func attachSemantic(srv *server.Server, cfg env.Config) error {
	if cfg.Embedding == nil {
		return nil
	}
	backend := memory.NewOllama(cfg.Embedding.OllamaURL, cfg.Embedding.Model)
	ctx, cancel := context.WithTimeout(context.Background(), semanticProbeTimeout)
	defer cancel()
	model, err := backend.Model(ctx)
	if err != nil {
		return err
	}
	store, err := openVectorStore(cfg, model)
	if err != nil {
		return err
	}
	srv.Semantic = &server.Semantic{Backend: backend, Store: store}
	return nil
}

func openVectorStore(cfg env.Config, model memory.ModelID) (memory.Store, error) {
	if cfg.Embedding.VectorStore == env.VectorStoreQdrant {
		return memory.OpenQdrantStore(cfg.Embedding.QdrantURL, qdrantCollection, model)
	}
	return memory.OpenFlatStore(filepath.Join(cfg.DataDir, vectorIndexDir), model)
}

var serveCmd = &cobra.Command{
	Use:          "serve",
	Short:        "Run sync server with web UI API",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := env.Load()
		if err != nil {
			logger.New(logger.Config{}).Error("failed to load config", slog.Any("error", err))
			os.Exit(1)
		}
		if dataDir, _ := cmd.Flags().GetString("data"); dataDir != "" {
			cfg.DataDir = dataDir
		}
		if cmd.Flags().Changed("port") {
			cfg.Port = servePort
		}

		var journalClient *journal.Client
		appLogger := logger.New(logger.Config{
			Level: cfg.LogLevel,
			Wrap: func(handler slog.Handler) slog.Handler {
				if cfg.JournalURL == "" || cfg.JournalToken == "" {
					return handler
				}
				journalClient = journal.New(journal.Config{URL: cfg.JournalURL, Token: cfg.JournalToken})
				return journal.NewHandler(journalClient, handler)
			},
		})
		if journalClient != nil {
			defer journalClient.Close()
		}

		srv := server.New(cfg.DataDir, cfg.Password)
		srv.SSOOnly = cfg.SSOOnly
		srv.OIDC = cfg.OIDC
		srv.CORSAllowedOrigins = cfg.CORSAllowedOrigins
		srv.Log = appLogger

		if err := attachSemantic(srv, cfg); err != nil {
			appLogger.Error("semantic search unavailable, memory search stays lexical", slog.Any("error", err))
		}
		if worker := server.NewEmbedWorker(srv); worker != nil {
			go worker.Run(cmd.Context())
		}

		emitter := server.NewEmitter(srv)
		go emitter.Run(cmd.Context())

		router := srv.Handler()
		clientDir := spa.DirFromEnv()
		if spa.Available(clientDir) {
			router.Handle("/*", spa.Handler(spa.Config{Dir: clientDir}))
			appLogger.Info("serving client", slog.String("dir", clientDir))
		}

		if cfg.Password == "" && cfg.OIDC == nil {
			appLogger.Warn("no authentication configured, every request is served as admin (set PASSWORD or OIDC_ISSUER)")
		}

		addr := ":" + strconv.Itoa(cfg.Port)
		appLogger.Info("server starting",
			slog.String("addr", addr),
			slog.String("data_dir", cfg.DataDir),
			slog.String("env", string(cfg.AppEnv)))
		return http.ListenAndServe(addr, router)
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", env.DefaultPort, "Port to listen on (default: $PORT)")
	serveCmd.Flags().String("data", "", "Data directory (default: $DATA_DIR, else ~/.mycelium/)")
	rootCmd.AddCommand(serveCmd)
}
