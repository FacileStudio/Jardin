package cmd

import (
	"fmt"
	"time"

	"github.com/FacileStudio/Mycelium/internal/cell"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show machine, sync state, and content summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}

		color.New(color.Bold).Printf("Machine: ")
		if cfg.Machine != "" {
			fmt.Println(cfg.Machine)
		} else {
			fmt.Println("not set")
		}

		color.New(color.Bold).Printf("Sync:    ")
		if url := cfg.ServerURL(); url != "" {
			fmt.Println(url)
		} else {
			fmt.Println("not configured")
		}

		color.New(color.Bold).Printf("Space:   ")
		if space := cfg.SpaceID(); space != "" {
			fmt.Println(space)
		} else {
			fmt.Println("common")
		}

		rules, _ := cell.ListRules()
		skills, _ := cell.ListSkills()

		fmt.Println()
		color.New(color.Bold).Printf("Rules:   ")
		fmt.Printf("%d\n", len(rules))
		color.New(color.Bold).Printf("Skills:  ")
		fmt.Printf("%d\n", len(skills))

		blocks, _ := sessions.ReadBlocks(config.DataDir())
		week := sessions.Aggregate(blocks, time.Now().Add(-7*24*time.Hour), "agent")
		var weekSessions int
		var weekSeconds int64
		for _, r := range week {
			weekSessions += r.Sessions
			weekSeconds += r.Seconds
		}
		color.New(color.Bold).Printf("Work:    ")
		if weekSessions == 0 {
			fmt.Println("no sessions this week")
		} else {
			fmt.Printf("%d sessions, %s active this week\n",
				weekSessions, sessions.FormatDuration(time.Duration(weekSeconds)*time.Second))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
