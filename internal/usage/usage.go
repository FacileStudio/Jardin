package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SourceStatusLine and SourceOAuth name where a snapshot came from: Claude
// Code's status-line payload, or a direct read of the OAuth usage endpoint.
const (
	SourceStatusLine = "statusline"
	SourceOAuth      = "oauth"
)

// HistoryThrottle and HistoryDelta gate the append-only history: the status
// line runs on nearly every keystroke, so a sample lands only when the clock
// or a percentage has moved enough to be worth a line.
const (
	HistoryThrottle = 5 * time.Minute
	HistoryDelta    = 1.0
)

// StaleAfter bounds how long a recorded snapshot stays presentable as current.
// The status line renders only while the user is working, so a snapshot older
// than this means nobody is reporting — not that nothing is being spent.
const StaleAfter = 15 * time.Minute

// windowOrder is the canonical display order. Anything absent from it sorts
// after every known key, alphabetically, so an upstream addition still renders
// deterministically.
var windowOrder = []string{
	"five_hour",
	"seven_day",
	"seven_day_opus",
	"seven_day_sonnet",
	"seven_day_overage_included",
}

var windowLabels = map[string]string{
	"five_hour":                  "5-hour session",
	"seven_day":                  "Weekly",
	"seven_day_opus":             "Weekly (Opus)",
	"seven_day_sonnet":           "Weekly (Sonnet)",
	"seven_day_overage_included": "Weekly (incl. overage)",
}

var windowShort = map[string]string{
	"five_hour":                  "5h",
	"seven_day":                  "7d",
	"seven_day_opus":             "7d opus",
	"seven_day_sonnet":           "7d sonnet",
	"seven_day_overage_included": "7d+overage",
}

type Window struct {
	Key            string     `json:"key"`
	Label          string     `json:"label"`
	UsedPercentage float64    `json:"used_percentage"`
	ResetsAt       *time.Time `json:"resets_at,omitempty"`
}

type Snapshot struct {
	Machine   string    `json:"machine"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"`
	Model     string    `json:"model,omitempty"`
	Windows   []Window  `json:"windows"`
}

// WindowView and SnapshotView are Snapshot resolved against the clock at read
// time. Freshness is never persisted, for the same reason sessions never
// persists liveness: a recorded percentage is a claim about a moment, and once
// the window has rolled over the stored 68% is a lie that would keep being told
// until the user next opens Claude Code.
type WindowView struct {
	Key             string     `json:"key"`
	Label           string     `json:"label"`
	UsedPercentage  float64    `json:"used_percentage"`
	ResetsAt        *time.Time `json:"resets_at,omitempty"`
	ResetsInSeconds *int64     `json:"resets_in_seconds,omitempty"`
	Expired         bool       `json:"expired"`
}

type SnapshotView struct {
	Machine    string       `json:"machine"`
	UpdatedAt  time.Time    `json:"updated_at"`
	AgeSeconds int64        `json:"age_seconds"`
	Stale      bool         `json:"stale"`
	Source     string       `json:"source"`
	Model      string       `json:"model,omitempty"`
	Windows    []WindowView `json:"windows"`
}

// View derives freshness from the stored timestamps and now. It never mutates
// used_percentage: an expired window still reports what was last observed, and
// Expired is the flag that tells the client not to present it as current.
func (s Snapshot) View(now time.Time) SnapshotView {
	age := int64(now.Sub(s.UpdatedAt).Seconds())
	if age < 0 {
		age = 0
	}
	view := SnapshotView{
		Machine:    s.Machine,
		UpdatedAt:  s.UpdatedAt,
		AgeSeconds: age,
		Stale:      now.Sub(s.UpdatedAt) > StaleAfter,
		Source:     s.Source,
		Model:      s.Model,
		Windows:    make([]WindowView, 0, len(s.Windows)),
	}
	for _, w := range s.Windows {
		out := WindowView{
			Key:            w.Key,
			Label:          w.Label,
			UsedPercentage: w.UsedPercentage,
			ResetsAt:       w.ResetsAt,
		}
		if w.ResetsAt != nil {
			remaining := int64(w.ResetsAt.Sub(now).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			out.ResetsInSeconds = &remaining
			out.Expired = now.After(*w.ResetsAt)
		}
		view.Windows = append(view.Windows, out)
	}
	return view
}

// Resolve is View across every machine.
func Resolve(snapshots []Snapshot, now time.Time) []SnapshotView {
	out := make([]SnapshotView, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, s.View(now))
	}
	return out
}

type HistorySeries struct {
	Key    string     `json:"key"`
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

type HistoryReport struct {
	Labels []string        `json:"labels"`
	Series []HistorySeries `json:"series"`
}

// ErrNoRateLimits means the payload carried no subscription limits at all,
// which Claude Code does until the session's first API response. Callers still
// get whatever else the payload held so they can render something.
var ErrNoRateLimits = errors.New("payload carries no rate_limits")

// Label names a window for humans; unknown keys are humanized rather than
// dropped, so a new upstream bucket is still readable.
func Label(key string) string {
	if label, ok := windowLabels[key]; ok {
		return label
	}
	if key == "" {
		return "(unknown)"
	}
	return strings.ToUpper(key[:1]) + strings.ReplaceAll(key[1:], "_", " ")
}

// Short names a window for the one-line status string.
func Short(key string) string {
	if s, ok := windowShort[key]; ok {
		return s
	}
	return key
}

func windowRank(key string) int {
	for i, k := range windowOrder {
		if k == key {
			return i
		}
	}
	return len(windowOrder)
}

func sortWindows(windows []Window) {
	sort.SliceStable(windows, func(i, j int) bool {
		ri, rj := windowRank(windows[i].Key), windowRank(windows[j].Key)
		if ri != rj {
			return ri < rj
		}
		return windows[i].Key < windows[j].Key
	})
}

func epochToTime(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

type statusLineBucket struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type statusLinePayload struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	RateLimits map[string]statusLineBucket `json:"rate_limits"`
}

// ParseStatusLine reads the JSON blob Claude Code pipes to a status-line
// command. rate_limits and every bucket inside it are optional; resets_at
// crosses the wire as Unix epoch seconds.
func ParseStatusLine(r io.Reader) (Snapshot, error) {
	var payload statusLinePayload
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&payload); err != nil {
		return Snapshot{Source: SourceStatusLine}, fmt.Errorf("decode status line payload: %w", err)
	}
	snapshot := Snapshot{
		Source:  SourceStatusLine,
		Model:   payload.Model.DisplayName,
		Windows: []Window{},
	}
	if len(payload.RateLimits) == 0 {
		return snapshot, ErrNoRateLimits
	}
	for key, bucket := range payload.RateLimits {
		snapshot.Windows = append(snapshot.Windows, Window{
			Key:            key,
			Label:          Label(key),
			UsedPercentage: bucket.UsedPercentage,
			ResetsAt:       epochToTime(bucket.ResetsAt),
		})
	}
	sortWindows(snapshot.Windows)
	return snapshot, nil
}

func machineDir(dataDir, machine string) string {
	return filepath.Join(dataDir, "usage", machine)
}

func currentPath(dataDir, machine string) string {
	return filepath.Join(machineDir(dataDir, machine), "current.json")
}

func historyPath(dataDir, machine string, t time.Time) string {
	return filepath.Join(machineDir(dataDir, machine), t.UTC().Format("2006-01")+".jsonl")
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".current-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Record publishes the snapshot as this machine's current.json and, when the
// throttle allows, appends it to the month's history shard. One writer per
// machine, so both files ride the normal file sync without conflicts.
func Record(dataDir, machine string, s Snapshot) error {
	if machine == "" {
		return errors.New("machine name required")
	}
	s.Machine = machine
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now()
	}
	s.UpdatedAt = s.UpdatedAt.UTC().Truncate(time.Second)
	if s.Windows == nil {
		s.Windows = []Window{}
	}
	sortWindows(s.Windows)

	// Both ingest paths write the same file, and the OAuth path can answer from
	// a five-minute-old cache, so the newer observation always wins. History
	// still gets the sample either way: the shards are an audit trail of what
	// was observed when, not a view of the present.
	if stored, ok := ReadOne(dataDir, machine); !ok || s.UpdatedAt.After(stored.UpdatedAt) {
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := writeAtomic(currentPath(dataDir, machine), data); err != nil {
			return err
		}
	}

	path := historyPath(dataDir, machine, s.UpdatedAt)
	last, hasLast := lastSample(path)
	if hasLast && !worthRecording(last, s) {
		return nil
	}
	line, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// worthRecording keeps the history readable: a sample lands when a window has
// moved at least HistoryDelta points, when the window set changed, or when the
// last sample has aged past HistoryThrottle.
func worthRecording(last, next Snapshot) bool {
	if next.UpdatedAt.Sub(last.UpdatedAt) > HistoryThrottle {
		return true
	}
	if len(last.Windows) != len(next.Windows) {
		return true
	}
	previous := make(map[string]float64, len(last.Windows))
	for _, w := range last.Windows {
		previous[w.Key] = w.UsedPercentage
	}
	for _, w := range next.Windows {
		before, ok := previous[w.Key]
		if !ok {
			return true
		}
		if delta := w.UsedPercentage - before; delta >= HistoryDelta || delta <= -HistoryDelta {
			return true
		}
	}
	return false
}

// lastSample reads the final JSON line of a shard without loading the whole
// file: the status line calls this on nearly every keystroke.
func lastSample(path string) (Snapshot, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return Snapshot{}, false
	}
	const window = 8 << 10
	size := info.Size()
	offset := int64(0)
	if size > window {
		offset = size - window
	}
	buf := make([]byte, size-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return Snapshot{}, false
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var s Snapshot
		if json.Unmarshal([]byte(lines[i]), &s) == nil && !s.UpdatedAt.IsZero() {
			return s, true
		}
	}
	return Snapshot{}, false
}

func machines(dataDir string) []string {
	entries, err := os.ReadDir(filepath.Join(dataDir, "usage"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

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
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
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
		f.Close()
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

	for _, s := range samples {
		out.Labels = append(out.Labels, s.UpdatedAt.UTC().Format(time.RFC3339))
	}
	for _, key := range keys {
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
		out.Series = append(out.Series, series)
	}
	return out
}
