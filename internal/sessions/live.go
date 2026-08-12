package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StaleAfter bounds how long a machine's heartbeat stays trustworthy. Three
// ticks of grace absorbs a missed sync or a brief network blip without the
// dashboard flapping a working machine to offline.
const StaleAfter = 3 * LiveInterval

// LiveInterval is the cadence the daemon publishes liveness at.
const LiveInterval = 60 * time.Second

// LiveBlock is one open work block on a machine: project, agent, timing and
// running totals.
type LiveBlock struct {
	Project     string    `json:"project"`
	Agent       string    `json:"agent"`
	Branch      string    `json:"branch,omitempty"`
	Model       string    `json:"model,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	LastEventAt time.Time `json:"last_event_at"`
	Events      int       `json:"events"`
	TokensOut   int64     `json:"tokens_out"`
}

// LiveSnapshot is what one machine is working on right now, as published by
// its daemon.
type LiveSnapshot struct {
	Machine   string      `json:"machine"`
	UpdatedAt time.Time   `json:"updated_at"`
	Open      []LiveBlock `json:"open"`
}

// LiveEntry is a snapshot block resolved against the clock at read time.
// Liveness is never persisted: a machine that sleeps mid-session would
// otherwise advertise itself as working forever.
type LiveEntry struct {
	LiveBlock
	Machine       string `json:"machine"`
	Live          bool   `json:"live"`
	MachineOnline bool   `json:"machine_online"`
	IdleSeconds   int64  `json:"idle_seconds"`
}

func livePath(dataDir, machine string) string {
	return filepath.Join(machineDir(dataDir, machine), "live.json")
}

// writeLive publishes the machine's open blocks so other machines and the
// dashboard can see work in progress. One writer per path, so it rides the
// normal file sync without conflicts.
func writeLive(dataDir, machine string, state *ScanState, now time.Time) error {
	snapshot := LiveSnapshot{Machine: machine, UpdatedAt: now.UTC(), Open: []LiveBlock{}}
	for _, b := range state.Open {
		if b.Events < 1 {
			continue
		}
		snapshot.Open = append(snapshot.Open, LiveBlock{
			Project:     b.Project,
			Agent:       b.Agent,
			Branch:      b.Branch,
			Model:       b.Model,
			StartedAt:   b.StartedAt.UTC(),
			LastEventAt: b.EndedAt.UTC(),
			Events:      b.Events,
			TokensOut:   b.TokensOut,
		})
	}
	sort.Slice(snapshot.Open, func(i, j int) bool {
		return snapshot.Open[i].LastEventAt.After(snapshot.Open[j].LastEventAt)
	})

	if err := os.MkdirAll(machineDir(dataDir, machine), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(livePath(dataDir, machine), data, 0o644)
}

// ReadLive collects every machine's snapshot and resolves liveness against
// now. Blocks idle beyond the gap timeout, or from machines whose heartbeat
// has gone stale, are returned with Live false rather than dropped, so the UI
// can distinguish "nothing running" from "machine went away mid-session".
func ReadLive(dataDir string, now time.Time) ([]LiveEntry, error) {
	entries := []LiveEntry{}
	machines, err := os.ReadDir(filepath.Join(dataDir, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	for _, m := range machines {
		if !m.IsDir() || strings.HasPrefix(m.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(livePath(dataDir, m.Name()))
		if err != nil {
			continue
		}
		var snapshot LiveSnapshot
		if json.Unmarshal(data, &snapshot) != nil {
			continue
		}
		online := now.Sub(snapshot.UpdatedAt) < StaleAfter
		for _, b := range snapshot.Open {
			idle := now.Sub(b.LastEventAt)
			entries = append(entries, LiveEntry{
				LiveBlock:     b,
				Machine:       snapshot.Machine,
				Live:          online && idle < GapTimeout,
				MachineOnline: online,
				IdleSeconds:   int64(idle.Seconds()),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Live != entries[j].Live {
			return entries[i].Live
		}
		return entries[i].LastEventAt.After(entries[j].LastEventAt)
	})
	return entries, nil
}
