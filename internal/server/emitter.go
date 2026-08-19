package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	antenneclient "github.com/FacileStudio/antenne-client/go"

	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/FacileStudio/Mycelium/internal/usage"
)

const (
	emitterInterval = 30 * time.Second
	minEmitDuration = time.Minute

	// outageGrace is how long the bus may be unreachable before a failed
	// connect is reported as an error rather than a retry.
	//
	// Redeploying Antenne takes seconds and is the common case; escalating
	// immediately turned every deploy into red lines in a shared dashboard,
	// which is how a colour stops carrying information. Anything still
	// failing after this long is not a deploy.
	outageGrace = 2 * time.Minute
)

// Emitter publishes sealed session blocks to the Antenne as
// agent_session.created events. The shards are the durable outbox; the ledger
// records which block IDs already went out. Block IDs are deterministic, so a
// crash between emit and ledger write yields a duplicate that Sablier's
// idempotency ledger absorbs — never a lost or double-counted entry.
type Emitter struct {
	srv     *Server
	mu      sync.Mutex
	client  *antenneclient.Client
	confKey string
	kick    chan struct{}

	// downSince is when the bus first became unreachable, cleared on the
	// next successful connect. It is what separates a redeploy from an
	// outage; see outageGrace.
	downSince time.Time

	// gen names the current client. Its callbacks fire from the client's own
	// goroutines and can land after the emitter has already replaced it, so a
	// discarded client's parting "disconnected" would otherwise mark a healthy
	// successor as down and get it torn back down on the next tick.
	gen       int
	connected bool
	lastError string
	emitted   int
}

// EmitterStatus is the emitter's health as the settings page shows it.
type EmitterStatus struct {
	Connected          bool   `json:"connected"`
	LastError          string `json:"last_error,omitempty"`
	Emitted            int    `json:"emitted"`
	Pending            int    `json:"pending"`
	UsageAlertsPending int    `json:"usage_alerts_pending"`
}

// NewEmitter builds the single emitter for a server and wires it in.
func NewEmitter(srv *Server) *Emitter {
	e := &Emitter{srv: srv, kick: make(chan struct{}, 1)}
	srv.emitter = e
	return e
}

func (e *Emitter) Kick() {
	select {
	case e.kick <- struct{}{}:
	default:
	}
}

func (e *Emitter) Status() EmitterStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	status := EmitterStatus{Connected: e.connected, LastError: e.lastError, Emitted: e.emitted}
	settings := e.srv.loadSettings()
	ledger := e.loadLedger()
	status.Pending = len(pendingBlocks(e.allBlocks(), ledger, &settings.Antenne))
	status.UsageAlertsPending = len(pendingUsageAlerts(e.allSnapshots(), ledger, &settings.Antenne, time.Now()))
	return status
}

// allBlocks aggregates sealed sessions from the common tree and every space
// tree — machines sync into exactly one of them, but billing sees all work.
func (e *Emitter) allBlocks() []sessions.Block {
	blocks, _ := sessions.ReadBlocks(e.srv.DataDir)
	entries, err := os.ReadDir(filepath.Join(e.srv.DataDir, "spaces"))
	if err != nil {
		return blocks
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		spaceBlocks, err := sessions.ReadBlocks(filepath.Join(e.srv.DataDir, "spaces", entry.Name()))
		if err == nil {
			blocks = append(blocks, spaceBlocks...)
		}
	}
	return blocks
}

// allSnapshots is allBlocks for usage: a machine reports into exactly one tree,
// but the alert is about that machine's account, so every tree is read.
func (e *Emitter) allSnapshots() []usage.Snapshot {
	snapshots, _ := usage.ReadCurrent(e.srv.DataDir)
	entries, err := os.ReadDir(filepath.Join(e.srv.DataDir, "spaces"))
	if err != nil {
		return snapshots
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		spaceSnapshots, err := usage.ReadCurrent(filepath.Join(e.srv.DataDir, "spaces", entry.Name()))
		if err == nil {
			snapshots = append(snapshots, spaceSnapshots...)
		}
	}
	return snapshots
}

func (e *Emitter) Run(ctx context.Context) {
	ticker := time.NewTicker(emitterInterval)
	defer ticker.Stop()
	for {
		e.tick(ctx)
		select {
		case <-ctx.Done():
			e.disconnect()
			return
		case <-ticker.C:
		case <-e.kick:
		}
	}
}

func (e *Emitter) tick(ctx context.Context) {
	settings := e.srv.loadSettings()
	antenne := settings.Antenne

	if !antenne.Enabled {
		e.disconnect()
		return
	}

	key := antenne.Instance + "|" + antenne.Secret
	e.mu.Lock()
	needsConnect := e.client == nil || e.confKey != key || !e.connected
	e.mu.Unlock()
	if needsConnect {
		if err := e.connect(ctx, &antenne, key); err != nil {
			e.connectFailed("connect: " + err.Error())
			return
		}
	}
	e.emitPending(&antenne)
}

func (e *Emitter) ledgerPath() string {
	return filepath.Join(e.srv.DataDir, ".pool-ledger.json")
}

func (e *Emitter) loadLedger() map[string]string {
	ledger := make(map[string]string)
	data, err := os.ReadFile(e.ledgerPath())
	if err != nil {
		return ledger
	}
	if err := json.Unmarshal(data, &ledger); err != nil {
		e.srv.Log.Error("emitter: corrupt ledger", slog.String("path", e.ledgerPath()), slog.Any("error", err))
		return make(map[string]string)
	}
	return ledger
}

func (e *Emitter) saveLedger(ledger map[string]string) error {
	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	return os.WriteFile(e.ledgerPath(), data, 0o600)
}
