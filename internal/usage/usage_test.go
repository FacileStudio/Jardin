package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func snap(pct float64, at time.Time) Snapshot {
	return Snapshot{
		Source:    SourceStatusLine,
		UpdatedAt: at,
		Windows:   []Window{{Key: "five_hour", Label: Label("five_hour"), UsedPercentage: pct}},
	}
}

func samples(t *testing.T, dir, machine string, at time.Time) []Snapshot {
	t.Helper()
	data, err := os.ReadFile(historyPath(dir, machine, at))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []Snapshot
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var s Snapshot
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestViewDerivesFreshness(t *testing.T) {
	now := time.Date(2026, 8, 8, 13, 20, 42, 0, time.UTC)
	future := now.Add(107 * time.Minute)
	past := now.Add(-2 * time.Hour)

	s := Snapshot{
		Machine:   "lucy",
		UpdatedAt: now.Add(-42 * time.Second),
		Source:    SourceStatusLine,
		Windows: []Window{
			{Key: "five_hour", Label: Label("five_hour"), UsedPercentage: 68.4, ResetsAt: &future},
			{Key: "seven_day", Label: Label("seven_day"), UsedPercentage: 41.2, ResetsAt: &past},
			{Key: "seven_day_opus", Label: Label("seven_day_opus"), UsedPercentage: 3},
		},
	}
	v := s.View(now)

	if v.AgeSeconds != 42 {
		t.Fatalf("age_seconds %d", v.AgeSeconds)
	}
	if v.Stale {
		t.Fatal("42s old must not be stale")
	}
	if got := *v.Windows[0].ResetsInSeconds; got != 6420 {
		t.Fatalf("resets_in_seconds %d", got)
	}
	if v.Windows[0].Expired {
		t.Fatal("a future window is not expired")
	}
	if !v.Windows[1].Expired {
		t.Fatal("a window whose resets_at has passed must be expired")
	}
	if got := *v.Windows[1].ResetsInSeconds; got != 0 {
		t.Fatalf("expired window must clamp to 0, got %d", got)
	}
	if v.Windows[1].UsedPercentage != 41.2 {
		t.Fatal("expired must not back-fill used_percentage — it is the last observed value")
	}
	if v.Windows[2].ResetsInSeconds != nil || v.Windows[2].Expired {
		t.Fatal("an unknown resets_at yields no resets_in_seconds and expired=false")
	}

	if !s.View(now.Add(StaleAfter + time.Second)).Stale {
		t.Fatalf("older than %s must be stale", StaleAfter)
	}
	if s.View(now.Add(StaleAfter - time.Minute)).Stale {
		t.Fatal("inside the window must not be stale")
	}
}

// TestDerivedFieldsAreNeverPersisted is the whole point of computing freshness
// at read time: the four derived keys must exist in the API response and in
// none of the files on disk.
func TestDerivedFieldsAreNeverPersisted(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	resets := now.Add(time.Hour)
	s := Snapshot{
		UpdatedAt: now,
		Source:    SourceStatusLine,
		Windows:   []Window{{Key: "five_hour", Label: Label("five_hour"), UsedPercentage: 68.4, ResetsAt: &resets}},
	}
	if err := Record(dir, "lucy", s); err != nil {
		t.Fatal(err)
	}

	derived := []string{"age_seconds", "stale", "resets_in_seconds", "expired"}
	for _, name := range []string{"current.json", historyPath(dir, "lucy", now)} {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, "usage", "lucy", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range derived {
			if strings.Contains(string(data), key) {
				t.Fatalf("%s persisted %q — freshness must never hit disk", path, key)
			}
		}
	}

	stored, _ := ReadCurrent(dir)
	body, err := json.Marshal(Resolve(stored, now))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range derived {
		if !strings.Contains(string(body), key) {
			t.Fatalf("API response is missing %q: %s", key, body)
		}
	}
}

func TestParseOAuthFractionToPercent(t *testing.T) {
	s, err := parseOAuth([]byte(`{"object":"usage","five_hour":{"utilization":0.684,"resets_at":1765089001},"seven_day":{"utilization":0.412}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != SourceOAuth {
		t.Fatalf("source %q", s.Source)
	}
	if len(s.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %+v", s.Windows)
	}
	if got := s.Windows[0].UsedPercentage; got < 68.3 || got > 68.5 {
		t.Fatalf("utilization must scale to a percentage, got %v", got)
	}
	if s.Windows[1].ResetsAt != nil {
		t.Fatal("absent resets_at must stay nil")
	}
}
