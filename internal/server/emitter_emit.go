package server

import (
	"log/slog"
	"time"

	antenneclient "github.com/FacileStudio/antenne-client/go"
	enveloppe "github.com/FacileStudio/enveloppe/go"

	"github.com/FacileStudio/Jardin/internal/sessions"
)

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

func envelopeFor(b *sessions.Block, email string) enveloppe.Event[enveloppe.AgentSession] {
	return enveloppe.Event[enveloppe.AgentSession]{
		Version:  enveloppe.EventVersion,
		App:      enveloppe.AppJardin,
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
