package usage

import (
	"errors"
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

// Window is one quota bucket: how much of it is used and when it resets.
type Window struct {
	Key            string     `json:"key"`
	Label          string     `json:"label"`
	UsedPercentage float64    `json:"used_percentage"`
	ResetsAt       *time.Time `json:"resets_at,omitempty"`
}

// Snapshot is a machine's usage at one moment in time.
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

// SnapshotView is a Snapshot resolved against the clock at read time, with
// its age in seconds.
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

// HistorySeries is one group's values across a history's labels.
type HistorySeries struct {
	Key    string     `json:"key"`
	Label  string     `json:"label"`
	Values []*float64 `json:"values"`
}

// HistoryReport is the answer to a history query: labels with one series per
// group.
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
