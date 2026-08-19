package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	antenneclient "github.com/FacileStudio/antenne-client/go"
)

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
		antenneclient.WithOnError(func(err error) { e.clientError(gen, err) }),
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

// setConnected flips the connectivity flag for the current generation, and
// when the bus comes back it clears the outage clock so the next unrelated
// blip gets its own grace period instead of inheriting an old one.
func (e *Emitter) setConnected(gen int, v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.gen {
		return
	}
	e.connected = v
	if v {
		e.downSince = time.Time{}
	}
}

// clientError logs a client failure at a level that matches what it means: a
// reconnect the client will retry is the mechanism working, not an incident —
// an Antenne restart produces a handful of these and resolves itself in
// seconds, and keeping them at error level turned every deploy of the bus red.
// The terminal failure still lands at error.
func (e *Emitter) clientError(gen int, err error) {
	msg := err.Error()
	e.mu.Lock()
	stale := gen != e.gen
	if !stale {
		e.lastError = msg
	}
	e.mu.Unlock()
	if stale {
		return
	}

	var transient *antenneclient.TransientError
	if errors.As(err, &transient) {
		e.srv.Log.Warn("emitter: reconnecting",
			slog.Int("attempt", transient.Attempt),
			slog.Any("error", transient.Err))
		return
	}
	e.srv.Log.Error("emitter", slog.String("error", msg))
}

// connectFailed records a failed connect, at a level that depends on how long
// the bus has been unreachable. lastError is set either way, so the settings
// page shows the reason from the first failure regardless of level.
func (e *Emitter) connectFailed(msg string) {
	e.mu.Lock()
	e.lastError = msg
	if e.downSince.IsZero() {
		e.downSince = time.Now()
	}
	down := time.Since(e.downSince)
	e.mu.Unlock()

	if down < outageGrace {
		e.srv.Log.Warn("emitter: reconnecting", slog.String("error", msg))
		return
	}
	e.srv.Log.Error("emitter", slog.String("error", msg), slog.Duration("down_for", down.Round(time.Second)))
}

func (e *Emitter) setError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
	e.srv.Log.Error("emitter", slog.String("error", msg))
}
