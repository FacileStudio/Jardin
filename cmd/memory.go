package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/memory"
	"github.com/FacileStudio/Jardin/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const memorySearchTimeout = 3 * time.Second

var (
	memorySearchLimit   int
	memorySearchLocal   bool
	memorySearchVerbose bool
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage memory",
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search memory, best match first",
	Long: "Search memory, best match first.\n\n" +
		"The server's hybrid search answers when it can; anything that stops it — no server, " +
		"no token, a timeout, a model that is down — silently falls back to the local index, " +
		"so a search always returns results. Pass --verbose to see which half answered.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := searchMemory(strings.Join(args, " "))
		if err != nil {
			return err
		}
		printSearchResults(results)
		return nil
	},
}

// searchMemory answers from the server when it can and from the local index
// otherwise. Every remote failure is demoted to a fallback rather than
// returned: a search that errors because a machine somewhere is down is worse
// than a search that ranks lexically.
func searchMemory(query string) ([]memory.SearchResult, error) {
	if memorySearchLocal {
		return memory.SearchChunks(config.MemoryDir(), query)
	}
	results, err := searchServer(query)
	if err == nil {
		return results, nil
	}
	if memorySearchVerbose {
		ui.Hint("server search unavailable (%v); using the local index", err)
	}
	return memory.SearchChunks(config.MemoryDir(), query)
}

func searchServer(query string) ([]memory.SearchResult, error) {
	cfg, err := config.LoadJardinConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.SemanticEnabled() {
		return nil, errors.New("vector search is off (set vector_search: true in ~/.jardin.yml)")
	}
	if cfg.ServerURL() == "" || cfg.AuthToken() == "" {
		return nil, errors.New("no server configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), memorySearchTimeout)
	defer cancel()

	spinner := ui.NewSpinner(os.Stdout, "searching")
	spinner.Start()
	results, degraded, err := memory.SearchRemote(ctx, memory.RemoteSearch{
		BaseURL: cfg.ServerURL(),
		Token:   cfg.AuthToken(),
		SpaceID: cfg.Space,
		Query:   query,
		Limit:   memorySearchLimit,
	})
	spinner.Stop()
	if err != nil {
		return nil, err
	}
	if memorySearchVerbose && degraded {
		ui.Hint("server answered lexically: its vector half is unavailable")
	}
	return results, nil
}

func printSearchResults(results []memory.SearchResult) {
	if len(results) == 0 {
		fmt.Println("No results.")
		return
	}
	shown := results
	if memorySearchLimit > 0 && len(shown) > memorySearchLimit {
		shown = shown[:memorySearchLimit]
	}
	for _, r := range shown {
		color.New(color.FgCyan).Printf("%s:%d ", r.Path, r.Line)
		fmt.Println(r.Content)
	}
	if len(shown) < len(results) {
		fmt.Printf("\n%d more match(es); raise --limit to see them.\n", len(results)-len(shown))
	}
}

var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Show memory index",
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := memory.ReadIndex(config.MemoryDir())
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	},
}

func init() {
	memorySearchCmd.Flags().IntVar(&memorySearchLimit, "limit", 20, "Maximum results to print")
	memorySearchCmd.Flags().BoolVar(&memorySearchLocal, "local", false,
		"Search the local index without asking the server")
	memorySearchCmd.Flags().BoolVar(&memorySearchVerbose, "verbose", false,
		"Report which index answered and why")
	memoryCmd.AddCommand(memorySearchCmd)
	memoryCmd.AddCommand(memoryIndexCmd)
	rootCmd.AddCommand(memoryCmd)
}
