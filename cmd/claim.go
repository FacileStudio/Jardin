package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/FacileStudio/Mycelium/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// claimVerifyTimeout bounds the server round trip a claim is taken against. It
// is memorySearchTimeout's number for memorySearchTimeout's reason: a server
// that is slow or gone must cost one moment and then get out of the way, never
// stand between an agent and its work.
const claimVerifyTimeout = 3 * time.Second

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

func claimContext(cfg *config.MyceliumConfig) (claimIdentity, error) {
	id := claimIdentity{}
	id.machine = claimMachine
	if id.machine == "" {
		id.machine = cfg.Machine
	}
	if id.machine == "" {
		return id, fmt.Errorf("machine not set — run 'mycelium login <url>' or pass --machine")
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

// claimVerdict is the answer to "may I take this claim": who already holds the
// repo, if anyone, and whether that answer came from the server or from the
// last view sync happened to leave on disk.
type claimVerdict struct {
	holder   *sessions.ClaimEntry
	verified bool
	err      error
}

// checkClaim asks the server who holds id's project and falls back to the
// local claim files when it cannot.
//
// The fallback is why verified exists. Local files arrive on a daemon tick, so
// two agents on two machines can both read "no claims" for up to a minute and
// both take the repo — the collision this check exists to stop. The server sees
// both immediately, so its answer is the one worth refusing on. But mycelium is
// local-first and a lock that fails closed would stop the work every time a
// laptop was on a train, so an unreachable server downgrades the verdict rather
// than ending the command. Every remote failure lands in err, which is a note
// to print and never a reason to return early.
func checkClaim(cfg *config.MyceliumConfig, id claimIdentity) claimVerdict {
	entries, err := fetchServerClaims(cfg)
	if err != nil {
		entries = sessions.ReadClaimsLive(config.DataDir(), id.project, time.Now())
	}
	return claimVerdict{holder: claimHolder(entries, id), verified: err == nil, err: err}
}

// fetchServerClaims reads every claim the server knows about, scoped to the
// configured space so a space member is answered about their own repos rather
// than the common root's.
func fetchServerClaims(cfg *config.MyceliumConfig) ([]sessions.ClaimEntry, error) {
	if cfg.ServerURL() == "" || cfg.AuthToken() == "" {
		return nil, errors.New("no server configured")
	}
	target := cfg.ServerURL() + "/api/claims"
	if cfg.Space != "" {
		target += "?space_id=" + url.QueryEscape(cfg.Space)
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimVerifyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claims: %s", resp.Status)
	}
	var entries []sessions.ClaimEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// claimHolder returns the claim on id's project that belongs to someone else,
// or nil when the repo is free.
//
// It blocks on MachineOnline rather than Live deliberately. Live also requires
// the claim to have been touched within sessions.StaleAfter, three minutes, and
// only 'claim note' touches one — so an agent that claimed a repo and then did
// the work would stop holding it after three quiet minutes. That is exactly the
// lock-that-is-not-a-lock this check replaces. A claim from a machine that
// stopped heartbeating does not block, and a holder that finished without
// releasing is cleared with 'mycelium claim done'.
func claimHolder(entries []sessions.ClaimEntry, id claimIdentity) *sessions.ClaimEntry {
	for i := range entries {
		e := &entries[i]
		if !strings.EqualFold(e.Project, id.project) {
			continue
		}
		if e.Machine == id.machine && e.Agent == id.agent {
			continue
		}
		if e.MachineOnline {
			return e
		}
	}
	return nil
}

// claimTaken renders the refusal: who holds the repo, since when, and the one
// command that takes it back. An agent that is told only "refused" retries.
func claimTaken(e *sessions.ClaimEntry) error {
	state := "idle"
	if e.Live {
		state = "active"
	}
	return fmt.Errorf("%s is claimed by %s/%s (%s since %s): %s\n  take it over with 'mycelium claim done -p %s -m %s --agent %s'",
		e.Project, e.Machine, e.Agent, state, sessions.FormatDuration(time.Since(e.StartedAt)), e.Task,
		e.Project, e.Machine, e.Agent)
}

// claimStartCmd silences usage from inside RunE rather than on the command:
// argument validation runs before RunE, so a typo still gets the flag list,
// while "this repo is taken" — the one message an agent must actually read —
// is not buried under twelve lines of it.
var claimStartCmd = &cobra.Command{
	Use:   "start <task>",
	Short: "Claim the current task on this repo",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cfg, err := config.LoadMyceliumConfig()
		if err != nil {
			return err
		}
		id, err := claimContext(cfg)
		if err != nil {
			return err
		}
		verdict := checkClaim(cfg, id)
		if verdict.holder != nil && verdict.verified {
			return claimTaken(verdict.holder)
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
		if !verdict.verified {
			ui.Warn("unverified: the server did not answer (%v), so this claim was taken from the last synced view", verdict.err)
			if verdict.holder != nil {
				ui.ErrorHint("that view shows %s/%s on %s: %s", verdict.holder.Machine, verdict.holder.Agent, verdict.holder.Project, verdict.holder.Task)
			} else {
				ui.ErrorHint("another machine may have claimed %s since the last sync", id.project)
			}
		}
		return nil
	},
}

var claimNoteCmd = &cobra.Command{
	Use:   "note <text>",
	Short: "Append a line to the active claim's scratchpad",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadMyceliumConfig()
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
			return fmt.Errorf("no active claim on %s — run 'mycelium claim start' first", id.project)
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
		cfg, err := config.LoadMyceliumConfig()
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
			ui.Hint("No active claims.")
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
			ui.Hint("No active claims on this repo.")
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
				ui.Hint("(no notes yet)")
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	claimCmd.PersistentFlags().StringVarP(&claimProject, "project", "p", "", "Project/repo to claim (default: current repo)")
	claimCmd.PersistentFlags().StringVarP(&claimMachine, "machine", "m", "", "Machine name (default: config)")
	claimCmd.PersistentFlags().StringVar(&claimAgent, "agent", "", "Agent name (default: machine)")
	claimStartCmd.Flags().StringVarP(&claimBranch, "branch", "b", "", "Branch (default: current git branch)")
	claimStartCmd.Flags().StringVar(&claimBody, "body", "", "Initial scratchpad body")
	claimListCmd.Flags().BoolVar(&claimAll, "all", false, "Show claims across all repos")
	claimCmd.AddCommand(claimStartCmd, claimNoteCmd, claimDoneCmd, claimListCmd, claimShowCmd)
	rootCmd.AddCommand(claimCmd)
}
