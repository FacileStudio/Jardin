package server

import (
	"strings"
	"testing"
	"time"

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

// A subscription limit is per Anthropic account, not per machine: lucy and ruche
// signed into the same plan see the same window with the same resets_at, so one
// real crossing must produce one alert. Both machines report to this instance,
// so a machine-keyed identity double-alerted for every crossing.
func TestPendingUsageAlertsSameEmailAlertsOncePerTick(t *testing.T) {
	antenne := alertSettings(80)
	resets := at(u0.Add(3 * time.Hour))
	snapshots := []usage.Snapshot{
		mkSnapshot("lucy", u0, mkWindow("five_hour", 82.4, resets)),
		mkSnapshot("ruche", u0, mkWindow("five_hour", 82.4, resets)),
	}

	ledger := map[string]string{}
	pending := pendingUsageAlerts(snapshots, ledger, antenne, u0)
	if len(pending) != 1 {
		t.Fatalf("two machines on one account must alert once, got %d: %+v", len(pending), pending)
	}
	ledger[pending[0].LedgerKey()] = u0.Format(time.RFC3339)
	if len(ledger) != 1 {
		t.Fatalf("one alert must write one ledger entry: %+v", ledger)
	}
	if pending[0].Email != "yann@facile.studio" {
		t.Fatalf("alert must carry the resolved email: %q", pending[0].Email)
	}
	if pending[0].Machine != "lucy" {
		t.Fatalf("equal readings tie-break on the smallest machine name: %q", pending[0].Machine)
	}
	if again := pendingUsageAlerts(snapshots, ledger, antenne, u0.Add(time.Minute)); len(again) != 0 {
		t.Fatalf("the ledger must suppress the next tick, got %+v", again)
	}
}

// The in-tick seen set and the ledger are two mechanisms and only the ledger
// survives a tick boundary. Feeding the machines in separate ticks takes seen out
// of the picture entirely, so a pass here proves the ledger is doing the work.
func TestPendingUsageAlertsSameEmailAlertsOnceAcrossTicks(t *testing.T) {
	antenne := alertSettings(80)
	resets := at(u0.Add(3 * time.Hour))
	lucy := []usage.Snapshot{mkSnapshot("lucy", u0, mkWindow("five_hour", 82.4, resets))}
	ruche := []usage.Snapshot{mkSnapshot("ruche", u0.Add(time.Minute), mkWindow("five_hour", 84, resets))}
	ledger := map[string]string{}

	first := pendingUsageAlerts(lucy, ledger, antenne, u0)
	if len(first) != 1 {
		t.Fatalf("the first machine must alert, got %d", len(first))
	}
	ledger[first[0].LedgerKey()] = u0.Format(time.RFC3339)

	second := pendingUsageAlerts(ruche, ledger, antenne, u0.Add(time.Minute))
	if len(second) != 0 {
		t.Fatalf("the other machine on the same account must not alert again, got %+v", second)
	}
	if len(ledger) != 1 {
		t.Fatalf("still exactly one ledger entry: %+v", ledger)
	}

	rolled := []usage.Snapshot{mkSnapshot("ruche", u0.Add(4*time.Hour), mkWindow("five_hour", 88, at(u0.Add(8*time.Hour))))}
	rolledPending := pendingUsageAlerts(rolled, ledger, antenne, u0.Add(4*time.Hour))
	if len(rolledPending) != 1 {
		t.Fatalf("a new resets_at must re-arm the email-keyed alert, got %d", len(rolledPending))
	}
	if rolledPending[0].LedgerKey() == first[0].LedgerKey() {
		t.Fatal("the rolled-over window must not reuse the previous ledger key")
	}
}
