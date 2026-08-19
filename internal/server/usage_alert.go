package server

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	enveloppe "github.com/FacileStudio/enveloppe/go"

	"github.com/FacileStudio/Mycelium/internal/usage"
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
			alert, eligible := windowAlert(s, w, email, threshold)
			if !eligible {
				continue
			}
			out = collectAlert(out, group, ledger, alert)
		}
	}
	return out
}

// windowAlert builds the alert one window would raise, and reports whether it
// raises one at all: a window with no reset time, an expired one, or one still
// under the threshold is not eligible.
func windowAlert(s usage.Snapshot, w usage.WindowView, email string, threshold float64) (usageAlert, bool) {
	if w.ResetsAt == nil || w.Expired || w.UsedPercentage < threshold {
		return usageAlert{}, false
	}
	label := w.Label
	if label == "" {
		label = usage.Label(w.Key)
	}
	return usageAlert{
		Machine:        s.Machine,
		Email:          email,
		Window:         w.Key,
		WindowLabel:    label,
		UsedPercentage: w.UsedPercentage,
		Threshold:      threshold,
		ResetsAt:       *w.ResetsAt,
		Source:         s.Source,
	}, true
}

// collectAlert adds one alert to the pass, dropping it when the ledger has
// already sent that key and keeping only the highest reading when a key is
// raised twice within the same pass.
func collectAlert(out []usageAlert, group map[string]int, ledger map[string]string, alert usageAlert) []usageAlert {
	key := alert.LedgerKey()
	if _, done := ledger[key]; done {
		return out
	}
	if i, ok := group[key]; ok {
		if alert.supersedes(&out[i]) {
			out[i] = alert
		}
		return out
	}
	group[key] = len(out)
	return append(out, alert)
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
