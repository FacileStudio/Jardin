package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/adapter"
	"github.com/FacileStudio/Mycelium/internal/cell"
	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/daemon"
	"github.com/FacileStudio/Mycelium/internal/journal"
	"github.com/FacileStudio/Mycelium/internal/memory"
	hsync "github.com/FacileStudio/Mycelium/internal/sync"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check Mycelium installation health",
	RunE: func(cmd *cobra.Command, args []string) error {
		allGood := true
		check := func(label string, fn func() (string, bool)) {
			msg, ok := fn()
			icon := color.GreenString("✓")
			if !ok {
				icon = color.RedString("✗")
				allGood = false
			}
			fmt.Printf("  %s %-12s %s\n", icon, label+":", msg)
		}

		color.New(color.Bold).Println("Mycelium doctor")
		fmt.Println()

		dataDir := config.DataDir()

		check("data dir", func() (string, bool) {
			info, err := os.Stat(dataDir)
			if err != nil {
				if os.IsNotExist(err) {
					return "missing — run 'mycelium init'", false
				}
				return err.Error(), false
			}
			if !info.IsDir() {
				return "not a directory", false
			}
			f, err := os.CreateTemp(dataDir, ".doctor-write-test-*")
			if err != nil {
				return "not writable", false
			}
			f.Close()
			os.Remove(f.Name())
			return dataDir, true
		})

		cfg, err := config.LoadMyceliumConfig()
		check("config", func() (string, bool) {
			if err != nil {
				return fmt.Sprintf("invalid — %v", err), false
			}
			path := config.ConfigPath()
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return "not found — create ~/.mycelium.yml", false
			}
			return path, true
		})

		check("machine", func() (string, bool) {
			if cfg.Machine == "" {
				return "not set — run 'mycelium status'", false
			}
			return cfg.Machine, true
		})

		check("sync", func() (string, bool) {
			url := cfg.ServerURL()
			if url == "" {
				return "not configured — run 'mycelium login <url>'", false
			}

			client := &http.Client{Timeout: 5 * time.Second}
			req, err := http.NewRequest(http.MethodHead, url+"/api/health", nil)
			if err != nil {
				return fmt.Sprintf("bad URL — %v", err), false
			}
			if cfg.AuthToken() != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.AuthToken())
			}

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Sprintf("unreachable — %v", err), false
			}
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				return fmt.Sprintf("server error (HTTP %d)", resp.StatusCode), false
			}
			return url + " — reachable", true
		})

		check("space", func() (string, bool) {
			if cfg.Space == "" {
				return "common (personal)", true
			}
			return cfg.Space, true
		})

		rules, _ := cell.ListRules()
		skills, _ := cell.ListSkills()
		check("rules", func() (string, bool) {
			if len(rules) == 0 {
				return "none", false
			}
			return fmt.Sprintf("%d file(s)", len(rules)), true
		})
		check("skills", func() (string, bool) {
			if len(skills) == 0 {
				return "none", false
			}
			return fmt.Sprintf("%d file(s)", len(skills)), true
		})

		check("agents", func() (string, bool) {
			agents := cfg.Agents
			if len(agents) == 0 {
				agents = daemon.DetectAgents()
			}
			if len(agents) == 0 {
				return "none detected", false
			}

			var problems []string
			for _, agent := range agents {
				a, err := adapter.Get(agent)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: unknown", agent))
					continue
				}
				for _, p := range a.TargetPaths() {
					dir := filepath.Dir(p)
					if _, err := os.Stat(dir); os.IsNotExist(err) {
						problems = append(problems, fmt.Sprintf("%s: %s missing", agent, dir))
					}
				}
			}
			if len(problems) > 0 {
				return strings.Join(problems, "; "), false
			}
			return fmt.Sprintf("%d configured (%s)", len(agents), strings.Join(agents, ", ")), true
		})

		check("daemon", func() (string, bool) {
			if daemon.Installed() {
				return "installed", true
			}
			return "not installed — run 'mycelium daemon install'", false
		})

		check("conflicts", func() (string, bool) {
			pages := hsync.ConflictBackups(dataDir)
			if len(pages) > 0 {
				return fmt.Sprintf("%d conflict(s): %s", len(pages), strings.Join(pages, ", ")), false
			}
			return "none", true
		})

		// The wiki is English-only (rules/20-memory.md). doctor is where this
		// belongs rather than the sync path alone: the daemon shells out to
		// `mycelium sync` every 60s and discards its output on success, so a
		// warning printed there reaches nobody. doctor is read by a human and
		// by the mycelium-health flow.
		check("wiki language", func() (string, bool) {
			memoryDir := filepath.Join(dataDir, "memory")
			if _, err := os.Stat(memoryDir); err != nil {
				return "no wiki on this machine", true
			}
			findings, err := memory.ScanWiki(memoryDir)
			if err != nil {
				return err.Error(), false
			}
			if len(findings) > 0 {
				return fmt.Sprintf("%d French line(s), first at %s:%d — the wiki is English-only",
					len(findings), findings[0].Path, findings[0].Line), false
			}
			return "English only", true
		})

		check("last sync", func() (string, bool) {
			return lastSyncAge(dataDir, time.Now())
		})

		check("history", func() (string, bool) {
			return historyState(dataDir)
		})

		fmt.Println()
		if allGood {
			color.Green("All checks passed.")
		} else {
			color.Red("Some checks failed — review above.")
		}
		return nil
	},
}

// historyState reports whether memory can still be recovered on this machine.
//
// A commit failure is only ever a warning on a sync that still succeeds, which
// is deliberate: a full disk must not stop a machine reaching its own wiki. The
// cost is that recording can stop and nothing else notices, so this is where it
// has to be visible.
//
// A machine that has never started one is not a failure. Every install that
// predates the journal is in that state until its next sync, and calling that
// broken would make the check cry wolf on every one of them.
func historyState(dataDir string) (string, bool) {
	health, err := journal.Inspect(dataDir)
	if err != nil {
		return err.Error() + " — memory cannot be recovered until this is fixed", false
	}
	if !health.Started {
		return "not started yet — the next sync will begin one", true
	}
	return fmt.Sprintf("recording, last %s by %s",
		health.Last.When.Format("2006-01-02 15:04"), health.Last.Machine), true
}

// syncStaleAfter is how long a machine may go without a completed sync before
// doctor calls it a failure. The daemon syncs about every 60 seconds, so a full
// day means something is stopping it: no network, no token, or a refused
// bulk delete waiting for a human to accept it. This check used to pass at any
// age, so a machine could sit unsynced for a week and still report itself
// healthy, which is the state a guard that stops a sync can now create.
const syncStaleAfter = 24 * time.Hour

// lastSyncAge reports how long ago the base manifest was written and whether
// that is recent enough to call healthy. now is a parameter so the threshold is
// testable without waiting a day for it.
func lastSyncAge(dataDir string, now time.Time) (string, bool) {
	info, err := os.Stat(filepath.Join(dataDir, ".sync-base.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "never synced", false
		}
		return err.Error(), false
	}
	ago := now.Sub(info.ModTime()).Truncate(time.Second)
	if ago > syncStaleAfter {
		return fmt.Sprintf("%s ago, run 'mycelium sync' to see why", ago), false
	}
	return fmt.Sprintf("%s ago", ago), true
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
