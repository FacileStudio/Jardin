package server

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Jardin/internal/sessions"
	"github.com/FacileStudio/Jardin/internal/usage"
)

func TestThresholdAccessorClamps(t *testing.T) {
	for _, tc := range []struct {
		raw  float64
		want float64
	}{
		{0, defaultUsageThreshold},
		{-5, defaultUsageThreshold},
		{0.4, 1},
		{1, 1},
		{80, 80},
		{100, 100},
		{1000, 100},
	} {
		antenne := &AntenneSettings{UsageThreshold: tc.raw}
		if got := antenne.Threshold(); got != tc.want {
			t.Fatalf("Threshold(%v) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// The alert ledger shares .pool-ledger.json with session blocks, whose IDs are
// bare 16-hex. The usage: prefix is what keeps the two namespaces apart.
func TestUsageLedgerKeyCannotCollideWithBlockID(t *testing.T) {
	alert := usageAlert{Machine: "lucy", Email: "yann@facile.studio", Window: "five_hour", Threshold: 80, ResetsAt: u0.Add(3 * time.Hour)}
	key := alert.LedgerKey()
	if !strings.HasPrefix(key, "usage:") {
		t.Fatalf("ledger key must be namespaced: %s", key)
	}
	hex16 := regexp.MustCompile(`^[0-9a-f]{16}$`)
	if !hex16.MatchString(alert.ID()) {
		t.Fatalf("alert id must be 16 hex like a block id: %s", alert.ID())
	}
	block := mkBlock(alert.ID(), "lucy", u0)
	if block.ID == key {
		t.Fatal("a block id must never equal a usage ledger key")
	}
	ledger := map[string]string{block.ID: "x", key: "y"}
	if len(ledger) != 2 {
		t.Fatal("block id and usage key collided in the shared ledger")
	}
	if _, done := ledger[block.ID]; !done {
		t.Fatal("block lookup broke")
	}
	if pending := pendingBlocks([]sessions.Block{block}, ledger, alertSettings(80)); len(pending) != 0 {
		t.Fatal("the usage entry must not disturb block dedupe")
	}
}

func TestUsageEnvelopeShape(t *testing.T) {
	alert := usageAlert{
		Machine:        "lucy",
		Email:          "yann@facile.studio",
		Window:         "five_hour",
		WindowLabel:    usage.Label("five_hour"),
		UsedPercentage: 82.4,
		Threshold:      80,
		ResetsAt:       u0.Add(3 * time.Hour),
		Source:         usage.SourceStatusLine,
	}
	evt := usageEnvelopeFor(&alert)
	if evt.FacileID != "fac_"+alert.ID() || evt.Payload.FacileID != evt.FacileID {
		t.Fatalf("facile id mismatch: %s / %s", evt.FacileID, evt.Payload.FacileID)
	}
	if !strings.HasSuffix(evt.IdempotencyKey, alert.ID()) {
		t.Fatalf("idempotency key must be derived from the alert id: %s", evt.IdempotencyKey)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"object":"usage_alert"`,
		`"action":"created"`,
		`"app":"Jardin"`,
		`"window":"five_hour"`,
		`"window_label":"5-hour session"`,
		`"used_percentage":82.4`,
		`"threshold":80`,
		`"resets_at":"2026-08-08T15:00:00Z"`,
		`"user_email":"yann@facile.studio"`,
		`"source":"statusline"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("envelope json missing %s: %s", want, data)
		}
	}
}

// Alerts enabled with no usage data at all must be a silent no-op, not a nil
// deref and not an error logged on every tick.
func TestEmitterStatusWithoutUsageData(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	e := NewEmitter(srv)
	if err := srv.saveSettings(Settings{Antenne: *alertSettings(80)}); err != nil {
		t.Fatal(err)
	}
	status := e.Status()
	if status.UsageAlertsPending != 0 || status.Pending != 0 {
		t.Fatalf("no data must yield no pending work: %+v", status)
	}
	if len(e.allSnapshots()) != 0 {
		t.Fatal("an empty data dir must yield no snapshots")
	}
}

func TestEmitterStatusCountsPendingUsageAlerts(t *testing.T) {
	dir := t.TempDir()
	srv := New(dir, "pw")
	e := NewEmitter(srv)
	if err := srv.saveSettings(Settings{Antenne: *alertSettings(80)}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	live := alertSettings(80)
	live.EmitSince = now.Add(-time.Hour).Format(time.RFC3339)
	if err := srv.saveSettings(Settings{Antenne: *live}); err != nil {
		t.Fatal(err)
	}
	resets := now.Add(2 * time.Hour)
	snapshot := mkSnapshot("lucy", now, mkWindow("five_hour", 93, &resets))
	if err := usage.Record(dir, "lucy", snapshot); err != nil {
		t.Fatal(err)
	}
	if got := e.Status().UsageAlertsPending; got != 1 {
		t.Fatalf("expected 1 pending usage alert, got %d", got)
	}
}
