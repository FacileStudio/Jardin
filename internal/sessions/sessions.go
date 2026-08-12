package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	GapTimeout  = 15 * time.Minute
	SealTimeout = 30 * time.Minute
)

// Block is a sealed work span: one project/agent/machine over a continuous
// interval, with the token and cost accounting for everything in it.
type Block struct {
	ID         string    `json:"id"`
	Project    string    `json:"project"`
	Machine    string    `json:"machine"`
	Agent      string    `json:"agent"`
	Branch     string    `json:"branch,omitempty"`
	Model      string    `json:"model,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	Events     int       `json:"events"`
	TokensIn   int64     `json:"tokens_in"`
	TokensOut  int64     `json:"tokens_out"`
	CacheRead  int64     `json:"cache_read"`
	CacheWrite int64     `json:"cache_write"`
	CostInput  float64   `json:"cost_input"`
	CostOutput float64   `json:"cost_output"`
	CostTotal  float64   `json:"cost_total"`
}

func (b *Block) Duration() time.Duration {
	return b.EndedAt.Sub(b.StartedAt)
}

func (b *Block) computeID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s",
		b.Machine, b.Agent, b.Project, b.StartedAt.UTC().Format(time.RFC3339))))
	return hex.EncodeToString(sum[:])[:16]
}

func (b *Block) IdempotencyKey() string {
	return "jardin_agent_session_created_" + b.ID
}

// Event is one interaction in an agent session.
type Event struct {
	Time       time.Time
	Agent      string
	Project    string
	Branch     string
	Model      string
	TokensIn   int64
	TokensOut  int64
	CacheRead  int64
	CacheWrite int64
	CostInput  float64
	CostOutput float64
	CostTotal  float64
}

// FileState tracks how much of one session file has been consumed already, so
// a restart resumes rather than re-ingesting everything.
type FileState struct {
	Offset     int64    `json:"offset"`
	Size       int64    `json:"size"`
	RecentReqs []string `json:"recent_request_ids,omitempty"`
}

// ScanState is the persisted resume point of the scanner: per-file offsets,
// open blocks and the cwd-to-project map.
type ScanState struct {
	Files    map[string]*FileState `json:"files"`
	Open     map[string]*Block     `json:"open"`
	Projects map[string]string     `json:"projects"`
}

func newScanState() *ScanState {
	return &ScanState{
		Files:    make(map[string]*FileState),
		Open:     make(map[string]*Block),
		Projects: make(map[string]string),
	}
}

// fold merges chronologically sorted events into the open blocks, returning
// every block that a gap larger than GapTimeout sealed along the way.
func fold(state *ScanState, machine string, events []Event, now time.Time) []Block {
	sort.Slice(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })

	var sealed []Block
	for _, ev := range events {
		key := strings.ToLower(ev.Project) + "|" + ev.Agent
		open := state.Open[key]
		if open != nil && ev.Time.After(open.EndedAt.Add(GapTimeout)) {
			sealed = append(sealed, finalize(open))
			open = nil
		}
		if open == nil {
			state.Open[key] = &Block{
				Project:   ev.Project,
				Machine:   machine,
				Agent:     ev.Agent,
				StartedAt: ev.Time,
				EndedAt:   ev.Time,
			}
			open = state.Open[key]
		}
		if ev.Time.Before(open.StartedAt) {
			open.StartedAt = ev.Time
		}
		if ev.Time.After(open.EndedAt) {
			open.EndedAt = ev.Time
		}
		if ev.Branch != "" {
			open.Branch = ev.Branch
		}
		if ev.Model != "" {
			open.Model = ev.Model
		}
		open.Events++
		open.TokensIn += ev.TokensIn
		open.TokensOut += ev.TokensOut
		open.CacheRead += ev.CacheRead
		open.CacheWrite += ev.CacheWrite
		open.CostInput += ev.CostInput
		open.CostOutput += ev.CostOutput
		open.CostTotal += ev.CostTotal
	}

	for key, open := range state.Open {
		if now.Sub(open.EndedAt) > SealTimeout {
			sealed = append(sealed, finalize(open))
			delete(state.Open, key)
		}
	}
	return sealed
}

func finalize(b *Block) Block {
	out := *b
	out.StartedAt = out.StartedAt.UTC()
	out.EndedAt = out.EndedAt.UTC()
	out.ID = out.computeID()
	return out
}
