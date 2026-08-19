package usage

import (
	"errors"
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
