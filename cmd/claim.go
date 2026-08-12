package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/sessions"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	claimProject string
	claimBranch  string
	claimMachine string
	claimAgent   string
	claimBody    string
	claimAll     bool
)

var claimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim a task on a repo and keep its scratchpad (coordination layer)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

type claimIdentity struct {
	machine string
	agent   string
	project string
}

func claimContext(cfg *config.JardinConfig) (claimIdentity, error) {
	id := claimIdentity{}
	id.machine = claimMachine
	if id.machine == "" {
		id.machine = cfg.Machine
	}
	if id.machine == "" {
		return id, fmt.Errorf("machine not set — run 'jardin login <url>' or pass --machine")
	}
	id.agent = claimAgent
	if id.agent == "" {
		id.agent = id.machine
	}
	id.project = claimProject
	if id.project == "" {
		cwd, _ := os.Getwd()
		id.project = sessions.ResolveProject(cwd)
	}
	if id.project == "" {
		return id, fmt.Errorf("cannot resolve project — pass --project")
	}
	return id, nil
}

func currentBranch(cwd string) string {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	b := strings.TrimSpace(string(out))
	if b == "" || b == "HEAD" {
		return ""
	}
	return b
}

var claimStartCmd = &cobra.Command{
	Use:   "start <task>",
	Short: "Claim the current task on this repo",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}
		id, err := claimContext(cfg)
		if err != nil {
			return err
		}
		branch := claimBranch
		if branch == "" {
			cwd, _ := os.Getwd()
			branch = currentBranch(cwd)
		}
		now := time.Now()
		c := &sessions.Claim{
			Project:   id.project,
			Machine:   id.machine,
			Agent:     id.agent,
			Branch:    branch,
			Task:      strings.Join(args, " "),
			StartedAt: now,
			UpdatedAt: now,
			Body:      claimBody,
		}
		if err := sessions.SaveClaim(config.DataDir(), c); err != nil {
			return err
		}
		color.Green("Claimed %q on %s (%s/%s)", c.Task, id.project, id.machine, id.agent)
		return nil
	},
}

var claimNoteCmd = &cobra.Command{
	Use:   "note <text>",
	Short: "Append a line to the active claim's scratchpad",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}
		id, err := claimContext(cfg)
		if err != nil {
			return err
		}
		existing, err := sessions.LoadClaim(config.DataDir(), id.project, id.machine, id.agent)
		if err != nil {
			return err
		}
		if existing == nil {
			return fmt.Errorf("no active claim on %s — run 'jardin claim start' first", id.project)
		}
		if existing.Body != "" {
			existing.Body += "\n"
		}
		existing.Body += time.Now().Local().Format("15:04") + " — " + strings.Join(args, " ")
		existing.UpdatedAt = time.Now()
		if err := sessions.SaveClaim(config.DataDir(), existing); err != nil {
			return err
		}
		color.Green("Noted on %s", id.project)
		return nil
	},
}

var claimDoneCmd = &cobra.Command{
	Use:   "done",
	Short: "Release the claim on this repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}
		id, err := claimContext(cfg)
		if err != nil {
			return err
		}
		if err := sessions.ReleaseClaim(config.DataDir(), id.project, id.machine, id.agent); err != nil {
			return err
		}
		color.Green("Released claim on %s", id.project)
		return nil
	},
}

var claimListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show active claims (this repo by default, --all for every repo)",
	RunE: func(cmd *cobra.Command, args []string) error {
		project := claimProject
		if !claimAll && project == "" {
			cwd, _ := os.Getwd()
			project = sessions.ResolveProject(cwd)
		}
		entries := sessions.ReadClaimsLive(config.DataDir(), project, time.Now())
		if len(entries) == 0 {
			fmt.Println("No active claims.")
			return nil
		}
		for _, e := range entries {
			switch {
			case e.Live:
				color.New(color.FgGreen).Printf("● ")
			case e.MachineOnline:
				color.New(color.FgYellow).Printf("◐ ")
			default:
				fmt.Printf("○ ")
			}
			color.New(color.Bold).Printf("%s", e.Project)
			fmt.Printf("  %s/%s", e.Machine, e.Agent)
			if e.Branch != "" {
				fmt.Printf("  %s", e.Branch)
			}
			fmt.Printf("  since %s: %s", sessions.FormatDuration(time.Since(e.StartedAt)), e.Task)
			if e.Live {
				color.Green("  active")
			} else if e.MachineOnline {
				color.Yellow("  idle")
			} else {
				fmt.Println("  machine offline")
			}
		}
		return nil
	},
}

var claimShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the full task and scratchpad of claims on this repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		project := claimProject
		if project == "" {
			cwd, _ := os.Getwd()
			project = sessions.ResolveProject(cwd)
		}
		entries := sessions.ReadClaimsLive(config.DataDir(), project, time.Now())
		if len(entries) == 0 {
			fmt.Println("No active claims on this repo.")
			return nil
		}
		for _, e := range entries {
			color.New(color.Bold).Printf("%s/%s: %s", e.Machine, e.Agent, e.Task)
			if e.Branch != "" {
				fmt.Printf(" [%s]", e.Branch)
			}
			fmt.Println()
			if e.Body != "" {
				fmt.Println(e.Body)
			} else {
				fmt.Println("  (no notes yet)")
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	claimCmd.PersistentFlags().StringVarP(&claimProject, "project", "p", "", "project/repo to claim (default: current repo)")
	claimCmd.PersistentFlags().StringVarP(&claimMachine, "machine", "m", "", "machine name (default: config)")
	claimCmd.PersistentFlags().StringVar(&claimAgent, "agent", "", "agent name (default: machine)")
	claimStartCmd.Flags().StringVarP(&claimBranch, "branch", "b", "", "branch (default: current git branch)")
	claimStartCmd.Flags().StringVar(&claimBody, "body", "", "initial scratchpad body")
	claimListCmd.Flags().BoolVar(&claimAll, "all", false, "show claims across all repos")
	claimCmd.AddCommand(claimStartCmd, claimNoteCmd, claimDoneCmd, claimListCmd, claimShowCmd)
	rootCmd.AddCommand(claimCmd)
}
