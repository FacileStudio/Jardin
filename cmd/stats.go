package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/sessions"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	statsSince string
	statsBy    string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Aggregate agent session stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		since, err := sessions.ParseSince(statsSince, time.Now())
		if err != nil {
			return err
		}
		valid := false
		for _, k := range sessions.GroupKeys {
			if k == statsBy {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("invalid --by %q (choose: %s)", statsBy, strings.Join(sessions.GroupKeys, ", "))
		}

		blocks, err := sessions.ReadBlocks(config.DataDir())
		if err != nil {
			return err
		}
		rows := sessions.Aggregate(blocks, since, statsBy)
		if len(rows) == 0 {
			fmt.Println("No sessions in range.")
			return nil
		}

		color.New(color.Bold).Printf("%-24s %8s %10s %12s %12s\n", strings.Title(statsBy), "Sessions", "Active", "Tokens in", "Tokens out")
		var totalSec, totalIn, totalOut int64
		totalSessions := 0
		for _, r := range rows {
			fmt.Printf("%-24s %8d %10s %12s %12s\n",
				truncate(r.Key, 24), r.Sessions,
				sessions.FormatDuration(time.Duration(r.Seconds)*time.Second),
				sessions.FormatTokens(r.TokensIn), sessions.FormatTokens(r.TokensOut))
			totalSessions += r.Sessions
			totalSec += r.Seconds
			totalIn += r.TokensIn
			totalOut += r.TokensOut
		}
		color.New(color.Bold).Printf("%-24s %8d %10s %12s %12s\n", "Total", totalSessions,
			sessions.FormatDuration(time.Duration(totalSec)*time.Second),
			sessions.FormatTokens(totalIn), sessions.FormatTokens(totalOut))
		return nil
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	statsCmd.Flags().StringVar(&statsSince, "since", "7d", "Window: 7d, 30d, 12h, or all")
	statsCmd.Flags().StringVar(&statsBy, "by", "project", "Group by: project, machine, agent, branch, model")
	rootCmd.AddCommand(statsCmd)
}
