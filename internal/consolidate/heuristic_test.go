package consolidate

import (
	"testing"
	"time"
)

func ts(offsetMinutes int) time.Time {
	return time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC).Add(time.Duration(offsetMinutes) * time.Minute)
}

func TestProposeErrorFix(t *testing.T) {
	tests := []struct {
		name     string
		episodes []Episode
		want     int
		text     string
	}{
		{
			name: "error followed by fix within window",
			episodes: []Episode{{
				Timestamp: ts(0),
				Agent:     "pi",
				Text:      "build failed\nchecking output\nthe fix was a missing import",
			}},
			want: 1,
			text: "build failed",
		},
		{
			name: "error with no fix in window",
			episodes: []Episode{{
				Timestamp: ts(0),
				Agent:     "pi",
				Text:      "error: something broke\nline two\nline three\nline four\nline five\nline six",
			}},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Propose(tt.episodes)
			var hits []Candidate
			for _, c := range got {
				if c.Kind == KindErrorFix {
					hits = append(hits, c)
				}
			}
			if len(hits) != tt.want {
				t.Fatalf("got %d error-fix candidates, want %d", len(hits), tt.want)
			}
			if tt.want > 0 && hits[0].Text != tt.text {
				t.Fatalf("text = %q, want %q", hits[0].Text, tt.text)
			}
		})
	}
}

func TestProposeGotcha(t *testing.T) {
	episodes := []Episode{{
		Timestamp: ts(1),
		Agent:     "pi",
		Text:      "all fine here\ngotcha: the flag is off by default",
	}}
	got := Propose(episodes)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	c := got[0]
	if c.Kind != KindGotcha {
		t.Fatalf("kind = %v, want KindGotcha", c.Kind)
	}
	if c.Text != "gotcha: the flag is off by default" {
		t.Fatalf("text = %q", c.Text)
	}
	if len(c.EpisodeRefs) != 1 || c.EpisodeRefs[0] != "pi@2026-08-24T10:01:00Z" {
		t.Fatalf("refs = %v", c.EpisodeRefs)
	}
}

func TestProposeRepeatedFailure(t *testing.T) {
	sig := "panic: nil map"
	episodes := []Episode{
		{Timestamp: ts(0), Agent: "pi", Text: sig},
		{Timestamp: ts(30), Agent: "codex", Text: "context\n" + sig},
	}
	got := Propose(episodes)
	var hits []Candidate
	for _, c := range got {
		if c.Kind == KindRepeatedFailure {
			hits = append(hits, c)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("got %d repeated-failure candidates, want 1", len(hits))
	}
	if len(hits[0].EpisodeRefs) != 2 {
		t.Fatalf("refs = %v, want 2 entries", hits[0].EpisodeRefs)
	}
}

func TestProposeRepeatedFailureSingleTimestamp(t *testing.T) {
	episodes := []Episode{{Timestamp: ts(0), Agent: "pi", Text: "error: x\nerror: y"}}
	got := Propose(episodes)
	for _, c := range got {
		if c.Kind == KindRepeatedFailure {
			t.Fatalf("unexpected repeated-failure candidate for one timestamp: %+v", c)
		}
	}
}

func TestProposeNoiseOnly(t *testing.T) {
	episodes := []Episode{
		{Timestamp: ts(0), Agent: "pi", Text: "refactored the parser today"},
		{Timestamp: ts(1), Agent: "pi", Text: "wrote some tests, all green"},
		{Timestamp: ts(2), Agent: "pi", Text: "updated the README"},
	}
	got := Propose(episodes)
	if len(got) != 0 {
		t.Fatalf("noise produced %d candidates: %+v", len(got), got)
	}
}
