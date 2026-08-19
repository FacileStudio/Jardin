package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReadCurrent returns every machine's latest snapshot, sorted by machine. A
// missing usage dir yields an empty slice, never an error.
func ReadCurrent(dataDir string) ([]Snapshot, error) {
	out := []Snapshot{}
	for _, machine := range machines(dataDir) {
		data, err := os.ReadFile(currentPath(dataDir, machine))
		if err != nil {
			continue
		}
		var s Snapshot
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		if s.Machine == "" {
			s.Machine = machine
		}
		if s.Windows == nil {
			s.Windows = []Window{}
		}
		sortWindows(s.Windows)
		out = append(out, s)
	}
	return out, nil
}

// ReadOne returns a single machine's latest snapshot.
func ReadOne(dataDir, machine string) (Snapshot, bool) {
	data, err := os.ReadFile(currentPath(dataDir, machine))
	if err != nil {
		return Snapshot{}, false
	}
	var s Snapshot
	if json.Unmarshal(data, &s) != nil {
		return Snapshot{}, false
	}
	if s.Machine == "" {
		s.Machine = machine
	}
	sortWindows(s.Windows)
	return s, true
}

func readSamples(dataDir, machine string, since time.Time) []Snapshot {
	dir := machineDir(dataDir, machine)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Snapshot
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		out = append(out, snapshotsInFile(filepath.Join(dir, name), machine, since)...)
	}
	return out
}

// snapshotsInFile reads one sample shard, keeping the snapshots inside the
// window. A sample written before Snapshot carried a machine is stamped with
// the directory it was found in, which is the machine that wrote it.
func snapshotsInFile(path, machine string, since time.Time) []Snapshot {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Snapshot
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		var s Snapshot
		if json.Unmarshal(scanner.Bytes(), &s) != nil || s.UpdatedAt.IsZero() {
			continue
		}
		if !since.IsZero() && s.UpdatedAt.Before(since) {
			continue
		}
		if s.Machine == "" {
			s.Machine = machine
		}
		out = append(out, s)
	}
	return out
}

// History returns every recorded sample in range as one series per window key,
// ascending by time. Samples are irregular, so labels are the sample instants
// themselves and a window missing from a sample carries null.
func History(dataDir string, since time.Time, machine string) HistoryReport {
	out := HistoryReport{Labels: []string{}, Series: []HistorySeries{}}
	var samples []Snapshot
	for _, m := range machines(dataDir) {
		if machine != "" && m != machine {
			continue
		}
		samples = append(samples, readSamples(dataDir, m, since)...)
	}
	if len(samples) == 0 {
		return out
	}
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].UpdatedAt.Before(samples[j].UpdatedAt) })

	for _, s := range samples {
		out.Labels = append(out.Labels, s.UpdatedAt.UTC().Format(time.RFC3339))
	}
	for _, key := range windowKeysInOrder(samples) {
		out.Series = append(out.Series, seriesFor(key, samples))
	}
	return out
}

// windowKeysInOrder collects every window key any sample carries, ranked so
// the series come back in a stable, meaningful order rather than map order.
func windowKeysInOrder(samples []Snapshot) []string {
	var keys []string
	seen := make(map[string]bool)
	for _, s := range samples {
		for _, w := range s.Windows {
			if !seen[w.Key] {
				seen[w.Key] = true
				keys = append(keys, w.Key)
			}
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		ri, rj := windowRank(keys[i]), windowRank(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// seriesFor builds one window's series across every sample. A sample that does
// not carry this window leaves a nil, which the dashboard renders as a gap
// rather than as a zero.
func seriesFor(key string, samples []Snapshot) HistorySeries {
	series := HistorySeries{Key: key, Label: Label(key), Values: make([]*float64, len(samples))}
	for i, s := range samples {
		for _, w := range s.Windows {
			if w.Key == key {
				value := w.UsedPercentage
				series.Values[i] = &value
				break
			}
		}
	}
	return series
}
