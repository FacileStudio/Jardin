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

var u0 = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

func mkSnapshot(machine string, updated time.Time, windows ...usage.Window) usage.Snapshot {
	return usage.Snapshot{Machine: machine, UpdatedAt: updated, Source: usage.SourceStatusLine, Windows: windows}
}

func mkWindow(key string, pct float64, resets *time.Time) usage.Window {
	return usage.Window{Key: key, Label: usage.Label(key), UsedPercentage: pct, ResetsAt: resets}
}

func at(t time.Time) *time.Time { return &t }

func alertSettings(threshold float64) *AntenneSettings {
	return &AntenneSettings{
		Enabled:        true,
		UserEmail:      "yann@facile.studio",
		EmitSince:      u0.Add(-time.Hour).Format(time.RFC3339),
		UsageAlerts:    true,
		UsageThreshold: threshold,
	}
}

func TestPendingUsageAlerts(t *testing.T) {
	future := at(u0.Add(3 * time.Hour))
	past := at(u0.Add(-time.Minute))

	cases := []struct {
		name      string
		antenne   *AntenneSettings
		snapshots []usage.Snapshot
		ledger    map[string]string
		want      []string
	}{
		{
			name:      "crossing emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 82.4, future))},
			want:      []string{"five_hour"},
		},
		{
			name:      "disabled never emits",
			antenne:   &AntenneSettings{Enabled: true, UserEmail: "yann@facile.studio"},
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 99, future))},
		},
		{
			name:      "below threshold never emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 79.9, future))},
		},
		{
			name:      "exactly at threshold emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 80, future))},
			want:      []string{"five_hour"},
		},
		{
			name:      "expired window never emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 100, past))},
		},
		{
			name:      "window without resets_at never emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 95, nil))},
		},
		{
			name:      "snapshot older than emit watermark never emits",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0.Add(-2*time.Hour), mkWindow("five_hour", 95, future))},
		},
		{
			name:    "every window is evaluated",
			antenne: alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0,
				mkWindow("five_hour", 81, future),
				mkWindow("seven_day", 90, at(u0.Add(72*time.Hour))),
				mkWindow("seven_day_opus", 12, at(u0.Add(72*time.Hour))),
			)},
			want: []string{"five_hour", "seven_day"},
		},
		{
			name:      "stale snapshot is still eligible",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0.Add(-30*time.Minute), mkWindow("five_hour", 88, future))},
			want:      []string{"five_hour"},
		},
		{
			name:      "zero threshold behaves as 80",
			antenne:   alertSettings(0),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 12, future))},
		},
		{
			name:      "zero threshold still emits above 80",
			antenne:   alertSettings(0),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 81, future))},
			want:      []string{"five_hour"},
		},
		{
			name: "machine without a resolvable email never emits",
			antenne: &AntenneSettings{
				Enabled:       true,
				MachineEmails: map[string]string{"lucy": "yann@facile.studio"},
				UsageAlerts:   true,
			},
			snapshots: []usage.Snapshot{mkSnapshot("ruche", u0, mkWindow("five_hour", 99, future))},
		},
		{
			name:      "no snapshots at all is a no-op",
			antenne:   alertSettings(80),
			snapshots: nil,
		},
		{
			name:      "no windows at all is a no-op",
			antenne:   alertSettings(80),
			snapshots: []usage.Snapshot{mkSnapshot("lucy", u0)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := tc.ledger
			if ledger == nil {
				ledger = map[string]string{}
			}
			pending := pendingUsageAlerts(tc.snapshots, ledger, tc.antenne, u0)
			var got []string
			for _, a := range pending {
				got = append(got, a.Window)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("windows: got %v want %v", got, tc.want)
			}
		})
	}
}

// The alert must fire once per window instance, not once per tick: a naive
// percentage check would re-emit forever while the user stays above the line.
func TestPendingUsageAlertsEdgeTriggered(t *testing.T) {
	antenne := alertSettings(80)
	resets := u0.Add(3 * time.Hour)
	snapshots := []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 82.4, at(resets)))}
	ledger := map[string]string{}

	first := pendingUsageAlerts(snapshots, ledger, antenne, u0)
	if len(first) != 1 {
		t.Fatalf("crossing must emit once, got %d", len(first))
	}
	ledger[first[0].LedgerKey()] = u0.Format(time.RFC3339)

	later := []usage.Snapshot{mkSnapshot("lucy", u0.Add(time.Minute), mkWindow("five_hour", 91, at(resets)))}
	if again := pendingUsageAlerts(later, ledger, antenne, u0.Add(time.Minute)); len(again) != 0 {
		t.Fatalf("same window instance must not re-emit, got %+v", again)
	}

	rolled := []usage.Snapshot{mkSnapshot("lucy", resets.Add(time.Minute), mkWindow("five_hour", 85, at(resets.Add(5*time.Hour))))}
	rolledPending := pendingUsageAlerts(rolled, ledger, antenne, resets.Add(time.Minute))
	if len(rolledPending) != 1 {
		t.Fatalf("a new resets_at must re-arm the alert, got %d", len(rolledPending))
	}
	if rolledPending[0].LedgerKey() == first[0].LedgerKey() {
		t.Fatal("a rolled-over window must not reuse the previous ledger key")
	}
}

// Lowering the threshold is a deliberate user action, so it re-arms: the key
// includes the threshold, which is what makes that work.
func TestPendingUsageAlertsThresholdChangeReArms(t *testing.T) {
	resets := at(u0.Add(3 * time.Hour))
	snapshots := []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 62, resets))}

	high := pendingUsageAlerts(snapshots, map[string]string{}, alertSettings(80), u0)
	if len(high) != 0 {
		t.Fatalf("62%% must not alert at 80, got %+v", high)
	}

	lowered := alertSettings(60)
	first := pendingUsageAlerts(snapshots, map[string]string{}, lowered, u0)
	if len(first) != 1 {
		t.Fatalf("lowering the threshold must emit, got %d", len(first))
	}

	ledger := map[string]string{}
	at80 := pendingUsageAlerts([]usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 95, resets))}, ledger, alertSettings(80), u0)
	if len(at80) != 1 {
		t.Fatal("95% must alert at 80")
	}
	ledger[at80[0].LedgerKey()] = u0.Format(time.RFC3339)
	reArmed := pendingUsageAlerts([]usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 95, resets))}, ledger, alertSettings(60), u0)
	if len(reArmed) != 1 {
		t.Fatal("lowering the threshold must re-arm an already-emitted window")
	}
	if reArmed[0].LedgerKey() == at80[0].LedgerKey() {
		t.Fatal("ledger key must differ when the threshold differs")
	}
}

// An unattributable machine is skipped rather than emitted: an alert with no
// user_email says nobody's limit is nearly spent. The skip must not consume the
// window instance — the ledger stays untouched, so configuring an email later
// still alerts for the very same window.
func TestPendingUsageAlertsSkipDoesNotConsumeWindow(t *testing.T) {
	snapshots := []usage.Snapshot{mkSnapshot("ruche", u0, mkWindow("five_hour", 93, at(u0.Add(3*time.Hour))))}
	ledger := map[string]string{}

	unattributed := &AntenneSettings{Enabled: true, UsageAlerts: true, EmitSince: u0.Add(-time.Hour).Format(time.RFC3339),
		MachineEmails: map[string]string{"lucy": "yann@facile.studio"}}
	if pending := pendingUsageAlerts(snapshots, ledger, unattributed, u0); len(pending) != 0 {
		t.Fatalf("machine without an email must not emit, got %+v", pending)
	}
	if len(ledger) != 0 {
		t.Fatalf("a skipped window must not write a ledger entry: %+v", ledger)
	}

	attributed := *unattributed
	attributed.MachineEmails = map[string]string{"lucy": "yann@facile.studio", "ruche": "ops@facile.studio"}
	pending := pendingUsageAlerts(snapshots, ledger, &attributed, u0)
	if len(pending) != 1 {
		t.Fatalf("the same window must emit once an email resolves, got %d", len(pending))
	}
	if got := usageEnvelopeFor(&pending[0], attributed.EmailFor(pending[0].Machine)).Payload.UserEmail; got != "ops@facile.studio" {
		t.Fatalf("per-machine override must win: %s", got)
	}
}

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
	alert := usageAlert{Machine: "lucy", Window: "five_hour", Threshold: 80, ResetsAt: u0.Add(3 * time.Hour)}
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
		Window:         "five_hour",
		WindowLabel:    usage.Label("five_hour"),
		UsedPercentage: 82.4,
		Threshold:      80,
		ResetsAt:       u0.Add(3 * time.Hour),
		Source:         usage.SourceStatusLine,
	}
	evt := usageEnvelopeFor(&alert, "yann@facile.studio")
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
