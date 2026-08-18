package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
)

const runExtension = ".json"

// List returns every flow file under the flows directory, sorted by name. A
// file that fails to parse aborts the listing rather than disappearing from it,
// because a silently skipped flow is a flow nobody notices is broken.
func List() ([]*Flow, error) {
	dir := config.FlowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Flow{}, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}
	flows := make([]*Flow, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != Extension {
			continue
		}
		f, err := readFlow(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		flows = append(flows, f)
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].Name < flows[j].Name })
	return flows, nil
}

// Load reads a single flow by name.
func Load(name string) (*Flow, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	path := filepath.Join(config.FlowsDir(), name+Extension)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no flow named %q: %s does not exist", name, path)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	return readFlow(path)
}

// SaveRun writes a run artifact under the runs directory, sets the run's ID
// from its start time, and returns the path written. Runs are owner-only: they
// hold captured output and never leave the machine.
func SaveRun(r *Run) (string, error) {
	if err := validName(r.Flow); err != nil {
		return "", err
	}
	dir := filepath.Join(config.RunsDir(), r.Flow)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", dir, err)
	}
	r.ID = newRunID(r.StartedAt)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to encode run %s: %w", r.ID, err)
	}
	path := filepath.Join(dir, r.ID+runExtension)
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return "", fmt.Errorf("failed to secure %s: %w", path, err)
	}
	return path, nil
}

// ListRuns returns a flow's recorded runs, newest first. A limit above zero
// caps how many are returned; a flow that has never run yields no runs and no
// error.
func ListRuns(name string, limit int) ([]*Run, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	dir := filepath.Join(config.RunsDir(), name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Run{}, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}
	runs := make([]*Run, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != runExtension {
			continue
		}
		r, err := readRun(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return newer(runs[i], runs[j]) })
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

// LoadRun returns one recorded run of a flow. An empty runID selects the newest
// run the flow has.
func LoadRun(name, runID string) (*Run, error) {
	if runID == "" {
		return newestRun(name)
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	if err := validName(runID); err != nil {
		return nil, err
	}
	path := filepath.Join(config.RunsDir(), name, runID+runExtension)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("flow %q has no run %q", name, runID)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	return readRun(path)
}

func newestRun(name string) (*Run, error) {
	runs, err := ListRuns(name, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("flow %q has no recorded runs", name)
	}
	return runs[0], nil
}

func readFlow(path string) (*Flow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	f, err := Parse(path, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func readRun(path string) (*Run, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: invalid run artifact: %w", path, err)
	}
	base := filepath.Base(path)
	r.ID = strings.TrimSuffix(base, filepath.Ext(base))
	return &r, nil
}

func newRunID(started time.Time) string {
	return strings.ReplaceAll(started.UTC().Format(time.RFC3339Nano), ":", "-")
}

func newer(a, b *Run) bool {
	if a.StartedAt.Equal(b.StartedAt) {
		return a.ID > b.ID
	}
	return a.StartedAt.After(b.StartedAt)
}

func validName(name string) error {
	if name == "" || strings.HasPrefix(name, ".") || name != filepath.Base(filepath.Clean(name)) {
		return fmt.Errorf("invalid name %q", name)
	}
	return nil
}
