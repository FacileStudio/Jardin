package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	antenneclient "github.com/FacileStudio/antenne-client/go"
	enveloppe "github.com/FacileStudio/enveloppe/go"

	"github.com/FacileStudio/Mycelium/internal/sessions"
	"github.com/FacileStudio/Mycelium/internal/usage"
)

const (
	emitterInterval = 30 * time.Second
	minEmitDuration = time.Minute
)

// objectUsageAlert is declared here, not in enveloppe: that repo is a shared
// cross-repo contract consumed by Opus and Sablier, and adopting a usage object
// there is a follow-up decision, not a side effect of this change. ObjectType is
// a typed string on a generic Event, so a contract-shaped event needs no fork.
const objectUsageAlert = enveloppe.ObjectType("usage_alert")

// UsageAlert is the usage_alert.created payload, shaped like enveloppe's own
// payload types so it can move into the contract unchanged if it is adopted.
type UsageAlert struct {
	FacileID       string  `json:"facile_id"`
	Machine        string  `json:"machine"`
	Window         string  `json:"window"`
	WindowLabel    string  `json:"window_label"`
	UsedPercentage float64 `json:"used_percentage"`
	Threshold      float64 `json:"threshold"`
	ResetsAt       string  `json:"resets_at"`
	UserEmail      string  `json:"user_email"`
	Source         string  `json:"source"`
}

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

	// gen names the current client. Its callbacks fire from the client's own
	// goroutines and can land after the emitter has already replaced it, so a
	// discarded client's parting "disconnected" would otherwise mark a healthy
	// successor as down and get it torn back down on the next tick.
	gen       int
	connected bool
	lastError string
	emitted   int
}

type EmitterStatus struct {
	Connected          bool   `json:"connected"`
	LastError          string `json:"last_error,omitempty"`
	Emitted            int    `json:"emitted"`
	Pending            int    `json:"pending"`
	UsageAlertsPending int    `json:"usage_alerts_pending"`
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
			e.setError("connect: " + err.Error())
			return
		}
	}
	e.emitPending(&antenne)
}

func (e *Emitter) connect(ctx context.Context, antenne *AntenneSettings, key string) error {
	gen := e.disconnect()

	cfg := &antenneclient.Config{
		App:      "Mycelium",
		Instance: antenne.Instance,
		Secret:   antenne.Secret,
		Events: antenneclient.EventConfig{
			Emit: []string{"agent_session.created", "usage_alert.created"},
		},
	}
	client := antenneclient.NewClient(cfg,
		antenneclient.WithOnConnect(func() { e.setConnected(gen, true) }),
		antenneclient.WithOnDisconnect(func() { e.setConnected(gen, false) }),
		antenneclient.WithOnError(func(err error) { e.clientError(gen, err.Error()) }),
	)
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		client.Disconnect()
		return err
	}
	e.mu.Lock()
	e.client = client
	e.confKey = key
	e.connected = true
	e.lastError = ""
	e.mu.Unlock()
	e.srv.Log.Info("emitter: connected to antenne pool", slog.String("instance", antenne.Instance))
	return nil
}

// disconnect drops the current client and returns the generation the next one
// will carry, retiring every callback the old client has yet to fire.
func (e *Emitter) disconnect() int {
	e.mu.Lock()
	client := e.client
	e.client = nil
	e.connected = false
	e.gen++
	gen := e.gen
	e.mu.Unlock()
	if client != nil {
		client.Disconnect()
	}
	return gen
}

func (e *Emitter) setConnected(gen int, v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.gen {
		return
	}
	e.connected = v
}

func (e *Emitter) clientError(gen int, msg string) {
	e.mu.Lock()
	stale := gen != e.gen
	if !stale {
		e.lastError = msg
	}
	e.mu.Unlock()
	if stale {
		return
	}
	e.srv.Log.Error("emitter", slog.String("error", msg))
}

func (e *Emitter) setError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
	e.srv.Log.Error("emitter", slog.String("error", msg))
}

func (e *Emitter) emitPending(antenne *AntenneSettings) {
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return
	}

	ledger := e.loadLedger()
	if !e.emitBlocks(client, antenne, ledger) {
		return
	}
	e.emitUsageAlerts(client, antenne, ledger)
}

func (e *Emitter) emitBlocks(client *antenneclient.Client, antenne *AntenneSettings, ledger map[string]string) bool {
	pending := pendingBlocks(e.allBlocks(), ledger, antenne)

	for _, b := range pending {
		evt := envelopeFor(&b, antenne.EmailFor(b.Machine))
		if err := client.EmitNow("agent_session.created", evt); err != nil {
			e.setError("emit: " + err.Error())
			return false
		}
		ledger[b.ID] = time.Now().UTC().Format(time.RFC3339)
		if err := e.saveLedger(ledger); err != nil {
			e.setError("ledger: " + err.Error())
			return false
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
		e.srv.Log.Info("emitter: published sessions", slog.Int("published", len(pending)), slog.Int("total", count))
	}
	return true
}

func (e *Emitter) emitUsageAlerts(client *antenneclient.Client, antenne *AntenneSettings, ledger map[string]string) {
	pending := pendingUsageAlerts(e.allSnapshots(), ledger, antenne, time.Now())

	for _, a := range pending {
		evt := usageEnvelopeFor(&a)
		if err := client.EmitNow("usage_alert.created", evt); err != nil {
			e.setError("emit: " + err.Error())
			return
		}
		ledger[a.LedgerKey()] = time.Now().UTC().Format(time.RFC3339)
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
		e.mu.Unlock()
		e.srv.Log.Info("emitter: published usage alerts", slog.Int("published", len(pending)))
	}
}

// pendingBlocks selects sealed blocks that are attributable, at least a
// minute long, ended after the emit watermark, and not yet in the ledger,
// oldest first. Sub-minute blocks are excluded outright — they stay in local
// stats but never become billing noise.
func pendingBlocks(blocks []sessions.Block, ledger map[string]string, antenne *AntenneSettings) []sessions.Block {
	var since time.Time
	if antenne.EmitSince != "" {
		since, _ = time.Parse(time.RFC3339, antenne.EmitSince)
	}
	var out []sessions.Block
	for _, b := range blocks {
		if _, done := ledger[b.ID]; done {
			continue
		}
		if b.Duration() < minEmitDuration {
			continue
		}
		if !since.IsZero() && b.EndedAt.Before(since) {
			continue
		}
		if antenne.EmailFor(b.Machine) == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

// usageAlert is one window of one account caught above the threshold. ResetsAt
// is the window instance's identity: when the window rolls over, Anthropic hands
// back a new one, so the same account legitimately alerts again. It is an
// absolute instant from Anthropic, not now-plus-remaining, so two machines
// observing the same window agree on it to the second and need no quantization.
//
// Email, not Machine, is the account's identity in the key. A subscription limit
// is per Anthropic account, so two machines signed into the same plan observe the
// same window with the same resets_at — keying on the machine fired one alert per
// machine for a single real crossing. Two machines mapped to different emails
// still alert separately, which is correct: those are two different people to
// tell. Machine and UsedPercentage are the reading of one member of the account,
// picked by supersedes rather than by walk order.
type usageAlert struct {
	Machine        string
	Email          string
	Window         string
	WindowLabel    string
	UsedPercentage float64
	Threshold      float64
	ResetsAt       time.Time
	Source         string
}

func (a *usageAlert) ID() string {
	sum := sha256.Sum256([]byte("usage_alert|" + a.Email + "|" + a.Window + "|" +
		a.ResetsAt.UTC().Format(time.RFC3339) + "|" + strconv.FormatFloat(a.Threshold, 'f', -1, 64)))
	return hex.EncodeToString(sum[:])[:16]
}

// LedgerKey namespaces the entry so it can never collide with a block ID in the
// shared .pool-ledger.json.
func (a *usageAlert) LedgerKey() string {
	return "usage:" + a.ID()
}

func (a *usageAlert) IdempotencyKey() string {
	return "mycelium_usage_alert_created_" + a.ID()
}

// supersedes decides which snapshot's reading represents an account when several
// map to one alert. The highest percentage wins: the readings describe a single
// shared account, so the highest observed value is the closest to the truth, and
// a threshold decision must never rest on a lower stale reading when a higher one
// is at hand. Ties fall to the smallest machine name, which makes the payload
// deterministic instead of walk-order dependent.
func (a *usageAlert) supersedes(cur *usageAlert) bool {
	if a.UsedPercentage != cur.UsedPercentage {
		return a.UsedPercentage > cur.UsedPercentage
	}
	return a.Machine < cur.Machine
}

// pendingUsageAlerts is pendingBlocks for usage: it selects window crossings
// that have not already gone out. The threshold is edge-triggered on the window
// instance rather than the percentage, so a sustained 90% alerts once and a
// rollover re-arms. Expired windows are skipped because they have already rolled
// over — alerting on one reports history as news — and a window with no
// resets_at has no instance identity, so it would repeat every tick. A stale
// snapshot is still eligible: the crossing genuinely happened, and staleness
// only means nobody has reported since.
//
// Eligible snapshots are grouped by alert identity, so the several machines of
// one account collapse into one alert carrying the group's highest reading (see
// supersedes) rather than whichever the walk reached first. That in-tick grouping
// and the ledger are two mechanisms and neither replaces the other: grouping
// stops one key going out twice in a single pass — one machine reporting into two
// trees, or two machines sharing an email — while the ledger is the durable
// guarantee that holds across ticks and restarts.
//
// A snapshot whose machine has no resolvable email is skipped exactly as
// pendingBlocks skips one, and without a ledger entry: whose limit is nearly spent
// is the whole content of the alert, and burning the dedupe key on an event a
// consumer may discard would retire that window instance for good. Skipping
// instead leaves it eligible the moment an email is configured.
func pendingUsageAlerts(snapshots []usage.Snapshot, ledger map[string]string, antenne *AntenneSettings, now time.Time) []usageAlert {
	if !antenne.UsageAlerts {
		return nil
	}
	var since time.Time
	if antenne.EmitSince != "" {
		since, _ = time.Parse(time.RFC3339, antenne.EmitSince)
	}
	threshold := antenne.Threshold()
	group := make(map[string]int)
	var out []usageAlert
	for _, s := range snapshots {
		if !since.IsZero() && s.UpdatedAt.Before(since) {
			continue
		}
		email := antenne.EmailFor(s.Machine)
		if email == "" {
			continue
		}
		for _, w := range s.View(now).Windows {
			if w.ResetsAt == nil || w.Expired {
				continue
			}
			if w.UsedPercentage < threshold {
				continue
			}
			label := w.Label
			if label == "" {
				label = usage.Label(w.Key)
			}
			alert := usageAlert{
				Machine:        s.Machine,
				Email:          email,
				Window:         w.Key,
				WindowLabel:    label,
				UsedPercentage: w.UsedPercentage,
				Threshold:      threshold,
				ResetsAt:       *w.ResetsAt,
				Source:         s.Source,
			}
			key := alert.LedgerKey()
			if _, done := ledger[key]; done {
				continue
			}
			if i, ok := group[key]; ok {
				if alert.supersedes(&out[i]) {
					out[i] = alert
				}
				continue
			}
			group[key] = len(out)
			out = append(out, alert)
		}
	}
	return out
}

// usageEnvelopeFor takes the email off the alert rather than resolving it again:
// it is part of the dedupe key, so a payload email that disagreed with the key
// would be a silent contradiction.
func usageEnvelopeFor(a *usageAlert) enveloppe.Event[UsageAlert] {
	id := a.ID()
	return enveloppe.Event[UsageAlert]{
		Version:  enveloppe.EventVersion,
		App:      enveloppe.AppMycelium,
		Object:   objectUsageAlert,
		Action:   enveloppe.ActionCreated,
		FacileID: "fac_" + id,
		Payload: UsageAlert{
			FacileID:       "fac_" + id,
			Machine:        a.Machine,
			Window:         a.Window,
			WindowLabel:    a.WindowLabel,
			UsedPercentage: a.UsedPercentage,
			Threshold:      a.Threshold,
			ResetsAt:       a.ResetsAt.UTC().Format(time.RFC3339),
			UserEmail:      a.Email,
			Source:         a.Source,
		},
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey: a.IdempotencyKey(),
	}
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
