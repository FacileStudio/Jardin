package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Claim is an in-flight task lease plus its mutable scratchpad body. It is the
// coordination layer's "who owns what and how far along" signal: an active
// claim on a repo tells a second agent to stay away or take over deliberately,
// and the body carries the work-in-progress state a takeover or a master agent
// needs to pick up mid-task. Each claim is one file per (machine, agent), so it
// has a single writer and rides the normal file sync without conflicts.
type Claim struct {
	Project   string    `json:"project"`
	Machine   string    `json:"machine"`
	Agent     string    `json:"agent"`
	Branch    string    `json:"branch,omitempty"`
	Task      string    `json:"task"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body,omitempty"`
}

// ClaimEntry resolves a stored claim against the clock at read time, the same
// liveness model as live.json: never persisted, so a machine that sleeps
// mid-claim stops advertising itself as working.
type ClaimEntry struct {
	Claim
	Live          bool `json:"live"`
	MachineOnline bool `json:"machine_online"`
}

func claimsDir(dataDir string) string {
	return filepath.Join(dataDir, "claims")
}

func claimPath(dataDir, project, machine, agent string) string {
	return filepath.Join(claimsDir(dataDir), project, machine+"--"+agent+".json")
}

// SaveClaim persists a claim under its (project, machine, agent) key.
func SaveClaim(dataDir string, c *Claim) error {
	if c.Project == "" || c.Machine == "" || c.Agent == "" {
		return fmt.Errorf("claim requires project, machine and agent")
	}
	p := claimPath(dataDir, c.Project, c.Machine, c.Agent)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// LoadClaim reads one claim, or returns nil when none exists.
func LoadClaim(dataDir, project, machine, agent string) (*Claim, error) {
	data, err := os.ReadFile(claimPath(dataDir, project, machine, agent))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c Claim
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ReleaseClaim removes a claim, tolerating an absent one.
func ReleaseClaim(dataDir, project, machine, agent string) error {
	err := os.Remove(claimPath(dataDir, project, machine, agent))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadClaims returns every claim on disk.
func ReadClaims(dataDir string) ([]Claim, error) {
	var claims []Claim
	root := claimsDir(dataDir)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") || strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var c Claim
		if json.Unmarshal(data, &c) == nil && c.Project != "" {
			claims = append(claims, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].StartedAt.After(claims[j].StartedAt) })
	return claims, nil
}

// machineOnline resolves which machines currently publish a live heartbeat,
// from the snapshots ReadLive reads. A claim only counts as live while its
// machine is actually working.
func machineOnline(dataDir string, now time.Time) map[string]bool {
	online := map[string]bool{}
	machines, err := os.ReadDir(filepath.Join(dataDir, "sessions"))
	if err != nil {
		return online
	}
	for _, m := range machines {
		if !m.IsDir() || strings.HasPrefix(m.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(livePath(dataDir, m.Name()))
		if err != nil {
			continue
		}
		var snap LiveSnapshot
		if json.Unmarshal(data, &snap) != nil {
			continue
		}
		if now.Sub(snap.UpdatedAt) < StaleAfter {
			online[snap.Machine] = true
		}
	}
	return online
}

// ReadClaimsLive returns every claim (optionally for one project, matched
// case-insensitively) with liveness resolved against now. A claim whose
// machine heartbeat is stale drops to MachineOnline=false; a claim not touched
// within StaleAfter is idle rather than live.
func ReadClaimsLive(dataDir, project string, now time.Time) []ClaimEntry {
	claims, _ := ReadClaims(dataDir)
	online := machineOnline(dataDir, now)
	out := make([]ClaimEntry, 0, len(claims))
	for _, c := range claims {
		if project != "" && !strings.EqualFold(c.Project, project) {
			continue
		}
		e := ClaimEntry{Claim: c}
		e.MachineOnline = online[c.Machine]
		e.Live = e.MachineOnline && now.Sub(c.UpdatedAt) < StaleAfter
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}
