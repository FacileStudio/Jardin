package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/FacileStudio/Hive/internal/config"
	"github.com/FacileStudio/Hive/internal/server"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run sync server",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir, _ := cmd.Flags().GetString("data")
		if dataDir == "" {
			dataDir = config.CellsDir()
		}

		token := os.Getenv("HIVE_TOKEN")

		srv := server.New(dataDir, token)

		addr := fmt.Sprintf(":%d", servePort)
		color.Green("Hive sync server listening on %s", addr)
		color.Green("Data: %s", dataDir)
		if token != "" {
			fmt.Println("Auth: bearer token required")
		} else {
			color.Yellow("Auth: none (set HIVE_TOKEN to enable)")
		}

		return http.ListenAndServe(addr, srv.Handler())
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8420, "port to listen on")
	serveCmd.Flags().String("data", "", "data directory (default: ~/.hive/cells/)")
	rootCmd.AddCommand(serveCmd)
}
