package server

import (
	"testing"
	"time"

	"github.com/FacileStudio/Jardin/internal/usage"
)

// One alert stands for one account, so the group reports its highest reading:
// the machines describe a single shared limit and the maximum is closest to the
// truth. Ties fall to the smallest machine name so the payload does not depend on
// walk order.
func TestPendingUsageAlertsReportsHighestReadingInGroup(t *testing.T) {
	antenne := alertSettings(80)
	resets := at(u0.Add(3 * time.Hour))

	cases := []struct {
		name        string
		snapshots   []usage.Snapshot
		wantPct     float64
		wantMachine string
	}{
		{
			name: "the maximum wins over walk order",
			snapshots: []usage.Snapshot{
				mkSnapshot("lucy", u0, mkWindow("five_hour", 82, resets)),
				mkSnapshot("ruche", u0, mkWindow("five_hour", 85, resets)),
			},
			wantPct:     85,
			wantMachine: "ruche",
		},
		{
			name: "the maximum wins when it is walked first",
			snapshots: []usage.Snapshot{
				mkSnapshot("lucy", u0, mkWindow("five_hour", 91, resets)),
				mkSnapshot("ruche", u0, mkWindow("five_hour", 85, resets)),
			},
			wantPct:     91,
			wantMachine: "lucy",
		},
		{
			name: "a tie falls to the smallest machine name",
			snapshots: []usage.Snapshot{
				mkSnapshot("ruche", u0, mkWindow("five_hour", 88, resets)),
				mkSnapshot("lucy", u0, mkWindow("five_hour", 88, resets)),
			},
			wantPct:     88,
			wantMachine: "lucy",
		},
		{
			name: "a machine below the threshold does not drag the alert down",
			snapshots: []usage.Snapshot{
				mkSnapshot("lucy", u0, mkWindow("five_hour", 40, resets)),
				mkSnapshot("ruche", u0, mkWindow("five_hour", 84, resets)),
			},
			wantPct:     84,
			wantMachine: "ruche",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pending := pendingUsageAlerts(tc.snapshots, map[string]string{}, antenne, u0)
			if len(pending) != 1 {
				t.Fatalf("one account must yield one alert, got %d: %+v", len(pending), pending)
			}
			if pending[0].UsedPercentage != tc.wantPct {
				t.Fatalf("used_percentage: got %v want %v", pending[0].UsedPercentage, tc.wantPct)
			}
			if pending[0].Machine != tc.wantMachine {
				t.Fatalf("machine: got %q want %q", pending[0].Machine, tc.wantMachine)
			}
		})
	}
}

// Two machines mapped to different emails are two different people to tell, so
// the collapse must not go too far.
func TestPendingUsageAlertsDifferentEmailsAlertSeparately(t *testing.T) {
	antenne := alertSettings(80)
	antenne.MachineEmails = map[string]string{"ruche": "ops@facile.studio"}
	resets := at(u0.Add(3 * time.Hour))
	snapshots := []usage.Snapshot{
		mkSnapshot("lucy", u0, mkWindow("five_hour", 91, resets)),
		mkSnapshot("ruche", u0, mkWindow("five_hour", 91, resets)),
	}

	ledger := map[string]string{}
	pending := pendingUsageAlerts(snapshots, ledger, antenne, u0)
	if len(pending) != 2 {
		t.Fatalf("two accounts must alert twice, got %d: %+v", len(pending), pending)
	}
	for _, a := range pending {
		ledger[a.LedgerKey()] = u0.Format(time.RFC3339)
	}
	if len(ledger) != 2 {
		t.Fatalf("two alerts must write two distinct ledger entries: %+v", ledger)
	}
	if pending[0].Email == pending[1].Email {
		t.Fatalf("both alerts resolved to the same email: %q", pending[0].Email)
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
	if got := usageEnvelopeFor(&pending[0]).Payload.UserEmail; got != "ops@facile.studio" {
		t.Fatalf("per-machine override must win: %s", got)
	}
}
