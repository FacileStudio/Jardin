package usage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseStatusLine(t *testing.T) {
	payload := `{"model":{"display_name":"Opus 5"},"rate_limits":{
		"seven_day":{"used_percentage":41.2,"resets_at":1765289001},
		"five_hour":{"used_percentage":68.4,"resets_at":1765089001}}}`

	s, err := ParseStatusLine(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != SourceStatusLine {
		t.Fatalf("source %q", s.Source)
	}
	if s.Model != "Opus 5" {
		t.Fatalf("model %q", s.Model)
	}
	if len(s.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(s.Windows))
	}
	if s.Windows[0].Key != "five_hour" || s.Windows[1].Key != "seven_day" {
		t.Fatalf("windows out of canonical order: %+v", s.Windows)
	}
	if s.Windows[0].Label != "5-hour session" {
		t.Fatalf("label %q", s.Windows[0].Label)
	}
	if s.Windows[0].ResetsAt == nil {
		t.Fatal("resets_at dropped")
	}
	if got := s.Windows[0].ResetsAt.Format(time.RFC3339); got != time.Unix(1765089001, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("epoch conversion wrong: %s", got)
	}
	if s.Windows[0].ResetsAt.Location() != time.UTC {
		t.Fatal("resets_at must be UTC")
	}
}

func TestParseStatusLineWithoutRateLimits(t *testing.T) {
	s, err := ParseStatusLine(strings.NewReader(`{"model":{"display_name":"Opus 5"}}`))
	if !errors.Is(err, ErrNoRateLimits) {
		t.Fatalf("expected ErrNoRateLimits, got %v", err)
	}
	if s.Model != "Opus 5" {
		t.Fatalf("model must survive so the caller can still print a line, got %q", s.Model)
	}
	if s.Windows == nil {
		t.Fatal("windows must be empty, not nil")
	}
}

func TestParseStatusLineOptionalBucketFields(t *testing.T) {
	s, err := ParseStatusLine(strings.NewReader(`{"rate_limits":{"five_hour":{"used_percentage":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Windows[0].ResetsAt != nil {
		t.Fatal("absent resets_at must stay nil so it is omitted")
	}
}

func TestParseStatusLineGarbageDoesNotPanic(t *testing.T) {
	if _, err := ParseStatusLine(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected a decode error")
	}
}

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

func TestRecordThrottle(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		s    Snapshot
		want int
	}{
		{"first sample always lands", snap(10, base), 1},
		{"sub-point move is throttled", snap(10.4, base.Add(time.Minute)), 1},
		{"a full point lands", snap(11.4, base.Add(2*time.Minute)), 2},
		{"still throttled just under a point", snap(12.3, base.Add(3*time.Minute)), 2},
		{"stale sample lands regardless", snap(12.3, base.Add(10*time.Minute)), 3},
	} {
		if err := Record(dir, "lucy", tc.s); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := len(samples(t, dir, "lucy", base)); got != tc.want {
			t.Fatalf("%s: expected %d samples, got %d", tc.name, tc.want, got)
		}
	}

	current, ok := ReadOne(dir, "lucy")
	if !ok {
		t.Fatal("current.json missing")
	}
	if current.Machine != "lucy" {
		t.Fatalf("machine %q", current.Machine)
	}
	if current.Windows[0].UsedPercentage != 12.3 {
		t.Fatalf("current.json must always hold the latest, got %v", current.Windows[0].UsedPercentage)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage", "lucy", "current.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRecordNewWindowLands(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	if err := Record(dir, "lucy", snap(10, base)); err != nil {
		t.Fatal(err)
	}
	two := snap(10, base.Add(time.Minute))
	two.Windows = append(two.Windows, Window{Key: "seven_day", Label: Label("seven_day"), UsedPercentage: 5})
	if err := Record(dir, "lucy", two); err != nil {
		t.Fatal(err)
	}
	if got := len(samples(t, dir, "lucy", base)); got != 2 {
		t.Fatalf("a new window must land, got %d samples", got)
	}
}

func TestReadCurrentAndHistory(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().UTC().Truncate(time.Second)
	if err := Record(dir, "ruche", snap(50, base)); err != nil {
		t.Fatal(err)
	}
	if err := Record(dir, "lucy", snap(10, base)); err != nil {
		t.Fatal(err)
	}

	all, err := ReadCurrent(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Machine != "lucy" {
		t.Fatalf("expected machine-sorted snapshots, got %+v", all)
	}

	h := History(dir, base.Add(-time.Hour), "")
	if len(h.Labels) != 2 || len(h.Series) != 1 {
		t.Fatalf("history shape wrong: %+v", h)
	}
	if len(h.Series[0].Values) != len(h.Labels) {
		t.Fatal("values must align to labels")
	}

	only := History(dir, base.Add(-time.Hour), "lucy")
	if len(only.Labels) != 1 {
		t.Fatalf("machine filter ignored: %+v", only)
	}
}

func TestReadCurrentEmpty(t *testing.T) {
	got, err := ReadCurrent(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty data must yield an empty slice, got %+v", got)
	}
	h := History(t.TempDir(), time.Time{}, "")
	if h.Labels == nil || h.Series == nil {
		t.Fatal("empty history must marshal as [] not null")
	}
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

func TestRecordNeverLetsAnOlderSampleWin(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		s    Snapshot
		want float64
	}{
		{"first sample lands", snap(10, base), 10},
		{"newer wins", snap(20, base.Add(time.Minute)), 20},
		{"older is ignored", snap(99, base.Add(-time.Hour)), 20},
		{"equal timestamp is a no-op", snap(77, base.Add(time.Minute)), 20},
		{"newer still wins afterwards", snap(30, base.Add(2*time.Minute)), 30},
	} {
		if err := Record(dir, "lucy", tc.s); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		current, ok := ReadOne(dir, "lucy")
		if !ok {
			t.Fatalf("%s: current.json missing", tc.name)
		}
		if got := current.Windows[0].UsedPercentage; got != tc.want {
			t.Fatalf("%s: current.json holds %v, want %v", tc.name, got, tc.want)
		}
	}

	if len(samples(t, dir, "lucy", base)) < 3 {
		t.Fatal("history is an audit trail — a rejected sample must still be recorded")
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
