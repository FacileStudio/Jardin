package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
)

const runExtension = ".json"

// RunRetention is how many artifacts a flow keeps. Old runs are history, not
// state: the wiki holds what was learned, and nobody reads the four hundredth
// most recent gate run.
const RunRetention = 50

// List returns every flow file under the flows directory, sorted by name. A
// file that fails to parse is reported as an error AND left out of the slice,
// which stays populated: a broken flow must be loud, but it must not take every
// working flow down with it.
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
	var broken []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != Extension {
			continue
		}
		f, err := readFlow(filepath.Join(dir, entry.Name()))
		if err != nil {
			broken = append(broken, err.Error())
			continue
		}
		flows = append(flows, f)
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].Name < flows[j].Name })
	if len(broken) > 0 {
		return flows, fmt.Errorf("%s", strings.Join(broken, "; "))
	}
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
	if err := pruneRuns(dir); err != nil {
		return path, fmt.Errorf("wrote %s but could not prune old runs: %w", path, err)
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

// Scaffold writes a starter flow file and returns its path. The flow is not
// trusted by it: whoever reviews it pins it with Trust.
func Scaffold(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	path := filepath.Join(config.FlowsDir(), name+Extension)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("flow %q already exists at %s", name, path)
	}
	if err := os.MkdirAll(config.FlowsDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(template(name)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func template(name string) string {
	return fmt.Sprintf(`name: %s
description: ""

# Steps run in order through "sh -c". Keep "run" to a single invocation and put
# any logic in a script, so a bashism cannot break one machine and not another.
#
# A step can read an earlier step's output. "needs" binds an environment
# variable to "<step>.<field>", where field is stdout, stderr or exit_code:
#
#   - name: second
#     needs:
#       VALUE: first.stdout
#     run: echo "first said $VALUE"
steps:
  - name: first
    run: echo "replace me"
`, name)
}

// newestRun returns a flow's latest run. It decodes every artifact, which is
// bounded by RunRetention rather than by how long the flow has existed —
// sorting on the filename would be cheaper and wrong, since RFC3339Nano trims
// trailing zeros and does not sort lexicographically.
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

// pruneRuns keeps the most recent RunRetention artifacts for a flow and deletes
// the rest. Run artifacts are the only unbounded thing jardin writes: a flow on
// a five-minute schedule produces a hundred thousand files a year, and every
// ListRuns decodes all of them.
func pruneRuns(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var artifacts []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == runExtension {
			artifacts = append(artifacts, entry)
		}
	}
	if len(artifacts) <= RunRetention {
		return nil
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Name() > artifacts[j].Name() })
	for _, doomed := range artifacts[RunRetention:] {
		if err := os.Remove(filepath.Join(dir, doomed.Name())); err != nil {
			return err
		}
	}
	return nil
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
