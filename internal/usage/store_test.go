package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
