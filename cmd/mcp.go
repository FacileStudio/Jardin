package cmd

import (
	"github.com/FacileStudio/Mycelium/internal/mcpserver"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Serve memory search, flows, and artifact tools to an agent over MCP on stdio",
	Long: "Serve memory search, flows, and artifact tools to an agent over MCP on stdio.\n\n" +
		"Tools: search_memory, list_flows, run_flow, and publish_artifact. An agent launches this as a " +
		"subprocess and speaks JSON-RPC over its stdin and stdout, so nothing here prints to " +
		"the terminal.\n\n" +
		"Stdio, not a URL: run_flow executes shell commands on this machine, and the pin " +
		"deciding which flows may run is per machine. A human still has to accept each flow " +
		"with \"mycelium flow trust <name>\" before an agent can run it.",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.Serve(cmd.Context(), version)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
