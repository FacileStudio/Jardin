package sessions

import (
	"testing"
	"time"
)

func tlBlock(machine string, start time.Time, minutes int) Block {
	return Block{
		ID:        machine + start.Format(time.RFC3339),
		Project:   "p",
		Machine:   machine,
		Agent:     "claude",
		StartedAt: start.UTC(),
		EndedAt:   start.UTC().Add(time.Duration(minutes) * time.Minute),
		Events:    1,
		TokensOut: int64(minutes),
	}
}

func TestTimelineGapFillsAndLabels(t *testing.T) {
	now := time.Now().UTC()
	blocks := []Block{
		tlBlock("lucy", now.AddDate(0, 0, -2), 60),
		tlBlock("lucy", now, 30),
	}
	got := Timeline(blocks, now.AddDate(0, 0, -2), "day", "machine")

	if len(got.Labels) != 3 {
		t.Fatalf("expected 3 day labels, got %v", got.Labels)
	}
	if got.Labels[0] != now.AddDate(0, 0, -2).Format("2006-01-02") {
		t.Fatalf("first label %q is not the start of the range", got.Labels[0])
	}
	if len(got.Series) != 1 || got.Series[0].Key != "lucy" {
		t.Fatalf("expected one lucy series, got %+v", got.Series)
	}
	s := got.Series[0]
	if len(s.Seconds) != 3 {
		t.Fatalf("series must align to labels, got %d values", len(s.Seconds))
	}
	if s.Seconds[0] != 3600 || s.Seconds[1] != 0 || s.Seconds[2] != 1800 {
		t.Fatalf("gap fill wrong: %v", s.Seconds)
	}
	if s.Sessions[1] != 0 || s.Sessions[0] != 1 {
		t.Fatalf("sessions wrong: %v", s.Sessions)
	}
}

func TestTimelineMonthBuckets(t *testing.T) {
	now := time.Now().UTC()
	blocks := []Block{tlBlock("lucy", now.AddDate(0, -2, 0), 60)}
	got := Timeline(blocks, now.AddDate(0, -2, 0), "month", "machine")

	if got.Bucket != "month" {
		t.Fatalf("bucket not echoed: %q", got.Bucket)
	}
	if len(got.Labels) != 3 {
		t.Fatalf("expected 3 month labels, got %v", got.Labels)
	}
	for _, label := range got.Labels {
		if _, err := time.Parse("2006-01", label); err != nil {
			t.Fatalf("label %q is not YYYY-MM", label)
		}
	}
	if got.Series[0].Seconds[0] != 3600 {
		t.Fatalf("first month should hold the block: %v", got.Series[0].Seconds)
	}
}

func TestTimelineCapsSeriesIntoOther(t *testing.T) {
	now := time.Now().UTC()
	var blocks []Block
	for i := 0; i < 9; i++ {
		blocks = append(blocks, tlBlock(string(rune('a'+i)), now, 100-i*10))
	}
	got := Timeline(blocks, now.AddDate(0, 0, -1), "day", "machine")

	if len(got.Series) != MaxSeries {
		t.Fatalf("expected %d series, got %d", MaxSeries, len(got.Series))
	}
	last := got.Series[len(got.Series)-1]
	if last.Key != OtherKey {
		t.Fatalf("last series must be %q, got %q", OtherKey, last.Key)
	}
	var folded int
	for _, n := range last.Sessions {
		folded += n
	}
	if folded != 9-(MaxSeries-1) {
		t.Fatalf("Other should fold %d blocks, folded %d", 9-(MaxSeries-1), folded)
	}
	if got.Series[0].Key != "a" {
		t.Fatalf("series must rank by seconds, first is %q", got.Series[0].Key)
	}
}

func TestTimelineExactlySixSeriesKeepsAll(t *testing.T) {
	now := time.Now().UTC()
	var blocks []Block
	for i := 0; i < MaxSeries; i++ {
		blocks = append(blocks, tlBlock(string(rune('a'+i)), now, 10+i))
	}
	got := Timeline(blocks, now.AddDate(0, 0, -1), "day", "machine")
	if len(got.Series) != MaxSeries {
		t.Fatalf("expected %d series, got %d", MaxSeries, len(got.Series))
	}
	for _, s := range got.Series {
		if s.Key == OtherKey {
			t.Fatal("no fold expected at exactly the cap")
		}
	}
}

func TestTimelineTotalCollapsesToAll(t *testing.T) {
	now := time.Now().UTC()
	blocks := []Block{
		tlBlock("lucy", now, 60),
		tlBlock("ruche", now, 30),
	}
	got := Timeline(blocks, now.AddDate(0, 0, -1), "day", TotalKey)

	if len(got.Series) != 1 || got.Series[0].Key != AllKey {
		t.Fatalf("by=total must yield one %q series, got %+v", AllKey, got.Series)
	}
	last := len(got.Labels) - 1
	if got.Series[0].Seconds[last] != 5400 {
		t.Fatalf("total seconds wrong: %v", got.Series[0].Seconds)
	}
	if got.Series[0].Sessions[last] != 2 {
		t.Fatalf("total sessions wrong: %v", got.Series[0].Sessions)
	}
}

func TestTimelineEmptyIsNotNull(t *testing.T) {
	got := Timeline(nil, time.Time{}, "day", TotalKey)
	if got.Labels == nil || got.Series == nil {
		t.Fatal("labels and series must marshal as [] not null")
	}
}
