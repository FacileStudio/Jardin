package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/server"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run sync server with web UI API",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir, _ := cmd.Flags().GetString("data")
		if dataDir == "" {
			dataDir = config.DataDir()
		}

		password := os.Getenv("PASSWORD")

		srv := server.New(dataDir, password)
		emitter := server.NewEmitter(srv)
		go emitter.Run(cmd.Context())

		addr := fmt.Sprintf(":%d", servePort)
		color.Green("Mycelium server listening on %s", addr)
		color.Green("Data: %s", dataDir)
		if password != "" {
			fmt.Println("Auth: password required (login via /api/auth/login)")
		} else {
			color.Yellow("Auth: none (set PASSWORD to enable)")
		}

		return http.ListenAndServe(addr, srv.Handler())
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8420, "port to listen on")
	serveCmd.Flags().String("data", "", "data directory (default: ~/.mycelium/)")
	rootCmd.AddCommand(serveCmd)
}
