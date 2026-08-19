package cmd

import (
	"fmt"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var sessionsScanAll bool

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Show recent agent work sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		blocks, err := sessions.ReadBlocks(config.DataDir())
		if err != nil {
			return err
		}
		if len(blocks) == 0 {
			ui.Hint("No sessions recorded yet — run 'mycelium sessions scan'.")
			return nil
		}
		for _, b := range sessions.Recent(blocks, 20) {
			color.New(color.FgCyan).Printf("%s  ", b.EndedAt.Local().Format("Jan 02 15:04"))
			color.New(color.Bold).Printf("%-20s", b.Project)
			fmt.Printf(" %s", sessions.FormatDuration(b.Duration()))
			fmt.Printf("  %s/%s", b.Machine, b.Agent)
			if b.Branch != "" {
				fmt.Printf("  %s", b.Branch)
			}
			fmt.Printf("  %s out\n", sessions.FormatTokens(b.TokensOut))
		}
		return nil
	},
}

var sessionsLiveCmd = &cobra.Command{
	Use:   "live",
	Short: "Show sessions running right now across machines",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := sessions.ReadLive(config.DataDir(), time.Now())
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			ui.Hint("No sessions running.")
			return nil
		}
		for _, e := range entries {
			switch {
			case e.Live:
				color.New(color.FgGreen).Printf("● ")
			case e.MachineOnline:
				color.New(color.FgYellow).Printf("● ")
			default:
				fmt.Printf("○ ")
			}
			color.New(color.Bold).Printf("%-20s", e.Project)
			fmt.Printf(" %-14s", e.Machine+"/"+e.Agent)
			fmt.Printf(" %8s", sessions.FormatDuration(time.Since(e.StartedAt)))
			fmt.Printf(" %8s out", sessions.FormatTokens(e.TokensOut))
			switch {
			case e.Live:
				color.Green("  active")
			case e.MachineOnline:
				color.Yellow("  idle %dm", e.IdleSeconds/60)
			default:
				fmt.Println("  machine offline")
			}
		}
		return nil
	},
}

var sessionsScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Collect new agent activity into session blocks",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}
		machine := cfg.Machine
		if machine == "" {
			return fmt.Errorf("machine not set — run 'mycelium login <url>' or set machine in ~/.mycelium.yml")
		}
		scan := sessions.Scan
		if sessionsScanAll {
			scan = sessions.Rescan
		}
		res, err := scan(config.DataDir(), machine, sessions.DefaultClaudeDir(), time.Now())
		if err != nil {
			return err
		}
		color.Green("Scanned %d new events — %d sealed, %d open", res.Events, res.Sealed, res.Open)
		return nil
	},
}

func init() {
	sessionsScanCmd.Flags().BoolVar(&sessionsScanAll, "all", false, "Rebuild this machine's history from full transcripts")
	sessionsCmd.AddCommand(sessionsScanCmd, sessionsLiveCmd)
	rootCmd.AddCommand(sessionsCmd)
}
