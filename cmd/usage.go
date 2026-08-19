package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
	"github.com/FacileStudio/Jardin/internal/ui"
	"github.com/FacileStudio/Jardin/internal/usage"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	usageStatusLine bool
	usageLive       bool
	usageJSON       bool
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show Claude subscription limits across machines",
	Long: "Reports how much of each subscription window is spent. Snapshots are recorded by\n" +
		"`jardin usage --statusline`, which Claude Code invokes to render its status line.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if usageStatusLine {
			runStatusLine()
			return nil
		}
		machine := usageMachine()
		dataDir := config.DataDir()

		if usageLive {
			cfg, err := config.LoadJardinConfig()
			if err != nil {
				return err
			}
			snapshot, err := usage.FetchOAuth(context.Background(), dataDir, usage.ResolveToken(cfg.UsageToken))
			switch {
			case errors.Is(err, usage.ErrTokenRejected):
				ui.Warn("%v", err)
				ui.Hint("falling back to the snapshot Claude Code's status line recorded")
			case err != nil:
				return err
			default:
				if err := usage.Record(dataDir, machine, snapshot); err != nil {
					return err
				}
			}
		}

		snapshots, err := usage.ReadCurrent(dataDir)
		if err != nil {
			return err
		}
		views := usage.Resolve(snapshots, time.Now())
		if usageJSON {
			return printUsageJSON(views, machine)
		}
		if len(views) == 0 {
			ui.Hint("No usage recorded yet — run 'jardin install claude' so Claude Code's status line reports it.")
			return nil
		}
		for i, v := range views {
			if i > 0 {
				fmt.Println()
			}
			printSnapshot(v, v.Machine == machine)
		}
		ui.Hint("statusline = reported by Claude Code, no credential needed; oauth = optional cross-check via 'jardin usage login'")
		return nil
	},
}

var usageLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store a subscription usage token, read from stdin",
	Long: "Reads a token on stdin — never from a flag, which would land in your shell history and in `ps`.\n" +
		"The token must be a subscription OAuth token from `claude setup-token` (sk-ant-oat01-…);\n" +
		"a standard sk-ant-api03-… API key cannot read subscription limits.\n\n" +
		"Usage: claude setup-token | jardin usage login",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(usage.TokenNotice)
		fmt.Println()

		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<10))
		if err != nil {
			return err
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return errors.New("no token on stdin (try: claude setup-token | jardin usage login)")
		}
		if strings.HasPrefix(token, "sk-ant-api") {
			return errors.New("that is a standard API key, which cannot read subscription limits — use `claude setup-token`")
		}

		if err := usage.KeychainStore(token); err == nil {
			ui.Success("Token stored in the OS keychain under %q.", usage.KeychainService)
			return nil
		} else if !errors.Is(err, usage.ErrNoKeychain) {
			ui.Warn("%v", err)
		}

		if inDataDir(config.ConfigPath()) {
			return errors.New("refusing to write a token into the synced data directory")
		}
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}
		cfg.UsageToken = token
		if err := config.SaveJardinConfig(cfg); err != nil {
			return err
		}
		ui.Warn("No OS keychain available — the token was written in plaintext to %s (mode 0600).", config.ConfigPath())
		ui.Hint("that file is outside the synced data directory, so it never leaves this machine")
		return nil
	},
}

var usageLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the stored usage token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := usage.KeychainDelete(); err != nil && !errors.Is(err, usage.ErrNoKeychain) {
			return err
		}
		cfg, err := config.LoadJardinConfig()
		if err != nil {
			return err
		}
		if cfg.UsageToken != "" {
			cfg.UsageToken = ""
			if err := config.SaveJardinConfig(cfg); err != nil {
				return err
			}
		}
		ui.Success("Usage token forgotten.")
		if os.Getenv(usage.TokenEnv) != "" || os.Getenv(usage.TokenEnvAlt) != "" {
			ui.Warn("%s or %s is still set in this environment.", usage.TokenEnv, usage.TokenEnvAlt)
		}
		return nil
	},
}

// inDataDir guards the one invariant that cannot be a comment: nothing holding
// a credential may live under the tree Jardin syncs to the server.
func inDataDir(path string) bool {
	data, err := filepath.Abs(config.DataDir())
	if err != nil {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(data, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func usageMachine() string {
	if cfg, err := config.LoadJardinConfig(); err == nil && cfg.Machine != "" {
		return cfg.Machine
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "unknown"
	}
	return host
}

func printUsageJSON(snapshots []usage.SnapshotView, machine string) error {
	var payload any = snapshots
	for _, s := range snapshots {
		if s.Machine == machine {
			payload = s
			break
		}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// printSnapshot renders derived state, never the raw stored number on its own:
// a percentage shown as current when its window has already reset is the worst
// thing this command could do.
func printSnapshot(s usage.SnapshotView, self bool) {
	name := s.Machine
	if self {
		name += " (this machine)"
	}
	color.New(color.Bold).Print(name)
	age := "updated " + humanAgo(time.Duration(s.AgeSeconds)*time.Second)
	details := []string{s.Source, age}
	if s.Model != "" {
		details = append([]string{s.Model}, details...)
	}
	fmt.Printf("  %s", color.New(color.Faint).Sprint(strings.Join(details, " · ")))
	if s.Stale {
		color.New(color.FgYellow).Print("  stale — nobody has reported since")
	}
	fmt.Println()
	if len(s.Windows) == 0 {
		ui.Hint("no windows reported")
		return
	}
	width := 0
	for _, w := range s.Windows {
		if len(w.Label) > width {
			width = len(w.Label)
		}
	}
	for _, w := range s.Windows {
		fmt.Printf("  %-*s  ", width, w.Label)
		if w.Expired {
			fmt.Printf("%s %s  %s\n",
				color.New(color.Faint).Sprint(strings.Repeat("░", 20)),
				color.New(color.Faint).Sprintf("%5.1f%%", w.UsedPercentage),
				color.New(color.Faint).Sprintf("as of %s, window has since reset",
					humanAgo(time.Duration(s.AgeSeconds)*time.Second)))
			continue
		}
		fmt.Printf("%s %s", bar(w.UsedPercentage, 20), percentColor(w.UsedPercentage).Sprintf("%5.1f%%", w.UsedPercentage))
		if w.ResetsInSeconds != nil {
			fmt.Printf("  %s", color.New(color.Faint).Sprintf("resets in %s", humanUntil(time.Duration(*w.ResetsInSeconds)*time.Second)))
		}
		fmt.Println()
	}
}

func percentColor(pct float64) *color.Color {
	switch {
	case pct >= 90:
		return color.New(color.FgRed)
	case pct >= 70:
		return color.New(color.FgYellow)
	default:
		return color.New(color.FgGreen)
	}
}

func bar(pct float64, width int) string {
	filled := int(pct/100*float64(width) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return percentColor(pct).Sprint(strings.Repeat("█", filled)) +
		color.New(color.Faint).Sprint(strings.Repeat("░", width-filled))
}

func humanUntil(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func humanAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	return humanUntil(d) + " ago"
}

// runStatusLine is on the critical path of the user's prompt: it renders on
// nearly every keystroke, so it never fails the process and never prints a
// stack trace. A broken payload still yields a line.
func runStatusLine() {
	snapshot, parseErr := usage.ParseStatusLine(os.Stdin)
	if parseErr == nil {
		if machine := usageMachine(); machine != "" {
			usage.Record(config.DataDir(), machine, snapshot)
		}
	}
	fmt.Println(statusLineText(snapshot))
}

func statusLineText(s usage.Snapshot) string {
	parts := []string{"Jardin"}
	for _, w := range s.Windows {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", usage.Short(w.Key), w.UsedPercentage))
	}
	if s.Model != "" {
		parts = append(parts, s.Model)
	}
	return strings.Join(parts, " · ")
}

func init() {
	usageCmd.Flags().BoolVar(&usageStatusLine, "statusline", false, "Read Claude Code's status-line payload on stdin, record it, print one line")
	usageCmd.Flags().BoolVar(&usageLive, "live", false, "Fetch live limits from Anthropic's OAuth usage endpoint")
	usageCmd.Flags().BoolVar(&usageJSON, "json", false, "Emit the snapshot as JSON")
	usageCmd.AddCommand(usageLoginCmd, usageLogoutCmd)
	rootCmd.AddCommand(usageCmd)
}
