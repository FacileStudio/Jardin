package sessions

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ScanResult struct {
	Events int
	Sealed int
	Open   int
}

func Scan(dataDir, machine, claudeDir string, now time.Time) (*ScanResult, error) {
	release, err := lockScan(dataDir)
	if err != nil {
		return nil, err
	}
	defer release()
	return scanLocked(dataDir, machine, claudeDir, now)
}

func scanLocked(dataDir, machine, claudeDir string, now time.Time) (*ScanResult, error) {
	state := LoadState(dataDir)
	resolve := projectResolver(state)

	events, err := collectClaude(claudeDir, state, resolve)
	if err != nil {
		return nil, err
	}
	sealed := fold(state, machine, events, now)
	if err := appendBlocks(dataDir, machine, sealed); err != nil {
		return nil, err
	}
	if err := SaveState(dataDir, state); err != nil {
		return nil, err
	}
	return &ScanResult{Events: len(events), Sealed: len(sealed), Open: len(state.Open)}, nil
}

// Rescan drops this machine's shards and scan state, then rebuilds from the
// full transcript history. Block IDs are deterministic, so downstream
// consumers deduplicate re-emitted history instead of double-counting it.
func Rescan(dataDir, machine, claudeDir string, now time.Time) (*ScanResult, error) {
	release, err := lockScan(dataDir)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := os.RemoveAll(machineDir(dataDir, machine)); err != nil {
		return nil, err
	}
	if err := os.Remove(statePath(dataDir)); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return scanLocked(dataDir, machine, claudeDir, now)
}

func projectResolver(state *ScanState) func(cwd string) string {
	home, _ := os.UserHomeDir()
	return func(cwd string) string {
		if p, ok := state.Projects[cwd]; ok {
			return p
		}
		p := resolveProject(cwd, home)
		state.Projects[cwd] = p
		return p
	}
}

func resolveProject(cwd, home string) string {
	if home != "" && filepath.Clean(cwd) == filepath.Clean(home) {
		return "home"
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if root := strings.TrimSpace(string(out)); root != "" {
			return filepath.Base(root)
		}
	}
	return filepath.Base(cwd)
}

func ResolveProject(cwd string) string {
	home, _ := os.UserHomeDir()
	return resolveProject(cwd, home)
}

func DefaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}
