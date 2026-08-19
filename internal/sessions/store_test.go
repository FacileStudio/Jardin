package sessions

import (
	"encoding/json"
	"testing"
	"time"
)

func TestShardRoundTrip(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		finalize(&Block{Project: "Jardin", Machine: "lucy", Agent: "claude", StartedAt: t0, EndedAt: t0.Add(10 * time.Minute), Events: 3, TokensOut: 500}),
		finalize(&Block{Project: "Sablier", Machine: "lucy", Agent: "claude", StartedAt: t0.Add(time.Hour), EndedAt: t0.Add(time.Hour + 5*time.Minute), Events: 2}),
	}
	if err := appendBlocks(dir, "lucy", blocks); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got))
	}
	if got[0].Project != "Jardin" || got[1].Project != "Sablier" {
		t.Fatalf("blocks out of order: %s, %s", got[0].Project, got[1].Project)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	state := newScanState()
	state.Files["/x.jsonl"] = &FileState{Offset: 42, Size: 100, RecentReqs: []string{"req_1"}}
	state.Projects["/Users/test/proj"] = "proj"
	fold(state, "lucy", []Event{ev(0, "Jardin", 10)}, t0.Add(time.Minute))
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	loaded := LoadState(dir)
	if loaded.Files["/x.jsonl"].Offset != 42 {
		t.Fatal("file state lost")
	}
	if loaded.Open["jardin|claude"] == nil {
		t.Fatal("open block lost")
	}
	data, _ := json.Marshal(loaded.Open["jardin|claude"])
	if len(data) == 0 {
		t.Fatal("open block must marshal")
	}
}

func TestAggregate(t *testing.T) {
	blocks := []Block{
		{Project: "Jardin", Machine: "lucy", StartedAt: t0, EndedAt: t0.Add(30 * time.Minute), TokensIn: 10, CacheWrite: 90, TokensOut: 100},
		{Project: "Jardin", Machine: "ruche", StartedAt: t0.Add(time.Hour), EndedAt: t0.Add(time.Hour + 30*time.Minute), TokensOut: 50},
		{Project: "Old", Machine: "lucy", StartedAt: t0.Add(-60 * 24 * time.Hour), EndedAt: t0.Add(-60 * 24 * time.Hour).Add(time.Hour)},
	}
	rows := Aggregate(blocks, t0.Add(-24*time.Hour), "project")
	if len(rows) != 1 {
		t.Fatalf("since filter failed: %d rows", len(rows))
	}
	if rows[0].Sessions != 2 || rows[0].Seconds != 3600 {
		t.Fatalf("bad row: %+v", rows[0])
	}
	if rows[0].TokensIn != 100 {
		t.Fatalf("tokens_in must include cache_write: %d", rows[0].TokensIn)
	}

	byMachine := Aggregate(blocks, time.Time{}, "machine")
	if len(byMachine) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(byMachine))
	}
}

func TestRecap(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{finalize(&Block{
		Project: "Jardin", Machine: "lucy", Agent: "claude", Branch: "main",
		StartedAt: t0, EndedAt: t0.Add(30 * time.Minute), Events: 5, TokensOut: 12000,
	})}
	if err := appendBlocks(dir, "lucy", blocks); err != nil {
		t.Fatal(err)
	}
	recap := Recap(dir, "Jardin", t0.Add(2*time.Hour))
	if recap == "" {
		t.Fatal("expected recap")
	}
	for _, want := range []string{"Jardin", "lucy", "branch main", "12.0k"} {
		if !contains(recap, want) {
			t.Fatalf("recap missing %q:\n%s", want, recap)
		}
	}
	if Recap(dir, "Unknown", t0) != "" {
		t.Fatal("unknown project must produce empty recap")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
