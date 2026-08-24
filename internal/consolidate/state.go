package consolidate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is the persisted outcome of the last run, read by doctor and used
// for the hourly rate limit. LastRun is stamped by any run that got as far as
// doing work, whether it succeeded or failed; a skip leaves it alone, because
// deciding not to run must never push the next chance an hour away.
type State struct {
	LastRun time.Time `json:"last_run"`
	Error   string    `json:"error,omitempty"`
	Result  *Result   `json:"result,omitempty"`
}

// StatePath is where the last-run state lives under the data dir.
func StatePath(dataDir string) string {
	return filepath.Join(dataDir, ".consolidate-run.json")
}

// LoadState reads the last-run state, returning nil when none exists yet.
func LoadState(dataDir string) (*State, error) {
	data, err := os.ReadFile(StatePath(dataDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func rateLimitSkip(state *State, opts Options) string {
	now := opts.Now
	if !opts.Force && now.Sub(state.LastRun) < hourlyLimit {
		return fmt.Sprintf("last run %s ago is inside the %s rate limit",
			now.Sub(state.LastRun).Truncate(time.Second), hourlyLimit)
	}
	return ""
}

func saveState(dataDir string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := StatePath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
