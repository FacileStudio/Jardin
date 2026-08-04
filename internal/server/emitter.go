package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	enveloppe "github.com/FacileStudio/enveloppe/go"
	pool "github.com/FacileStudio/pool/go"

	"github.com/FacileStudio/Mycelium/internal/sessions"
)

const emitterInterval = 30 * time.Second

// Emitter publishes sealed session blocks to the Nook pool as
// agent_session.created events. The shards are the durable outbox; the ledger
// records which block IDs already went out. Block IDs are deterministic, so a
// crash between emit and ledger write yields a duplicate that Sablier's
// idempotency ledger absorbs — never a lost or double-counted entry.
type Emitter struct {
	srv     *Server
	mu      sync.Mutex
	client  *pool.Client
	confKey string
	kick    chan struct{}

	connected bool
	lastError string
	emitted   int
}

type EmitterStatus struct {
	Connected bool   `json:"connected"`
	LastError string `json:"last_error,omitempty"`
	Emitted   int    `json:"emitted"`
	Pending   int    `json:"pending"`
}

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
	if blocks, err := sessions.ReadBlocks(e.srv.DataDir); err == nil {
		ledger := e.loadLedger()
		status.Pending = len(pendingBlocks(blocks, ledger, &settings.Nook))
	}
	return status
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
	nook := settings.Nook

	if !nook.Enabled {
		e.disconnect()
		return
	}

	key := nook.Instance + "|" + nook.Secret
	e.mu.Lock()
	needsConnect := e.client == nil || e.confKey != key || !e.connected
	e.mu.Unlock()
	if needsConnect {
		if err := e.connect(ctx, &nook, key); err != nil {
			e.setError("connect: " + err.Error())
			return
		}
	}
	e.emitPending(&nook)
}

func (e *Emitter) connect(ctx context.Context, nook *NookSettings, key string) error {
	e.disconnect()

	cfg := &pool.Config{
		App:      "Mycelium",
		Instance: nook.Instance,
		Secret:   nook.Secret,
		Events: pool.EventConfig{
			Emit: []string{"agent_session.created"},
		},
	}
	client := pool.NewClient(cfg,
		pool.WithOnConnect(func() { e.setConnected(true) }),
		pool.WithOnDisconnect(func() { e.setConnected(false) }),
		pool.WithOnError(func(err error) { e.setError(err.Error()) }),
	)
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		return err
	}
	e.mu.Lock()
	e.client = client
	e.confKey = key
	e.connected = true
	e.lastError = ""
	e.mu.Unlock()
	log.Printf("emitter: connected to nook pool at %s", nook.Instance)
	return nil
}

func (e *Emitter) disconnect() {
	e.mu.Lock()
	client := e.client
	e.client = nil
	e.connected = false
	e.mu.Unlock()
	if client != nil {
		client.Disconnect()
	}
}

func (e *Emitter) setConnected(v bool) {
	e.mu.Lock()
	e.connected = v
	e.mu.Unlock()
}

func (e *Emitter) setError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
	log.Printf("emitter: %s", msg)
}

func (e *Emitter) emitPending(nook *NookSettings) {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return
	}

	blocks, err := sessions.ReadBlocks(e.srv.DataDir)
	if err != nil {
		e.setError("read blocks: " + err.Error())
		return
	}
	ledger := e.loadLedger()
	pending := pendingBlocks(blocks, ledger, nook)

	for _, b := range pending {
		evt := envelopeFor(&b, nook.EmailFor(b.Machine))
		if err := client.EmitNow("agent_session.created", evt); err != nil {
			e.setError("emit: " + err.Error())
			return
		}
		ledger[b.ID] = time.Now().UTC().Format(time.RFC3339)
		if err := e.saveLedger(ledger); err != nil {
			e.setError("ledger: " + err.Error())
			return
		}
		e.mu.Lock()
		e.emitted++
		e.mu.Unlock()
	}
	if len(pending) > 0 {
		e.mu.Lock()
		e.lastError = ""
		count := e.emitted
		e.mu.Unlock()
		log.Printf("emitter: published %d session(s) (%d total)", len(pending), count)
	}
}

// pendingBlocks selects sealed blocks that are attributable, ended after the
// emit watermark, and not yet in the ledger, oldest first.
func pendingBlocks(blocks []sessions.Block, ledger map[string]string, nook *NookSettings) []sessions.Block {
	var since time.Time
	if nook.EmitSince != "" {
		since, _ = time.Parse(time.RFC3339, nook.EmitSince)
	}
	var out []sessions.Block
	for _, b := range blocks {
		if _, done := ledger[b.ID]; done {
			continue
		}
		if !since.IsZero() && b.EndedAt.Before(since) {
			continue
		}
		if nook.EmailFor(b.Machine) == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func envelopeFor(b *sessions.Block, email string) enveloppe.Event[enveloppe.AgentSession] {
	return enveloppe.Event[enveloppe.AgentSession]{
		Version:  enveloppe.EventVersion,
		App:      enveloppe.AppMycelium,
		Object:   enveloppe.ObjectAgentSession,
		Action:   enveloppe.ActionCreated,
		FacileID: "fac_" + b.ID,
		Payload: enveloppe.AgentSession{
			FacileID:  "fac_" + b.ID,
			Project:   b.Project,
			Machine:   b.Machine,
			Agent:     b.Agent,
			Branch:    b.Branch,
			UserEmail: email,
			StartedAt: b.StartedAt.UTC().Format(time.RFC3339),
			StoppedAt: b.EndedAt.UTC().Format(time.RFC3339),
			TokensIn:  b.TokensIn + b.CacheWrite,
			TokensOut: b.TokensOut,
		},
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey: b.IdempotencyKey(),
	}
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
		log.Printf("emitter: corrupt ledger %s: %v", e.ledgerPath(), err)
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
