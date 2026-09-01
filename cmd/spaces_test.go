package cmd

import (
	"testing"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"1234", "1234"},
		{"12345678", "12345678"},
		{"446b594d-fea8-4657-b424-7287cf86a3e2", "446b594d"},
	}

	for _, tt := range tests {
		got := shortID(tt.input)
		if got != tt.expected {
			t.Errorf("shortID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveSpace(t *testing.T) {
	spaces := []spaceInfo{
		{ID: "446b594d-fea8-4657-b424-7287cf86a3e2", Name: "FacileShared", Role: "owner"},
		{ID: "99887766-1122-3344-5566-778899aabbcc", Name: "AnotherSpace", Role: "member"},
		{ID: "446b9999-0000-0000-0000-000000000000", Name: "DuplicatePrefix", Role: "member"},
	}

	res, err := resolveSpace(spaces, "446b594d-fea8-4657-b424-7287cf86a3e2")
	if err != nil || res == nil || res.Name != "FacileShared" {
		t.Fatalf("expected exact ID match, got %v, err: %v", res, err)
	}

	res, err = resolveSpace(spaces, "99887766")
	if err != nil || res == nil || res.Name != "AnotherSpace" {
		t.Fatalf("expected prefix match, got %v, err: %v", res, err)
	}

	res, err = resolveSpace(spaces, "facileshared")
	if err != nil || res == nil || res.ID != "446b594d-fea8-4657-b424-7287cf86a3e2" {
		t.Fatalf("expected case-insensitive name match, got %v, err: %v", res, err)
	}

	_, err = resolveSpace(spaces, "446b")
	if err == nil {
		t.Fatalf("expected ambiguous prefix error, got nil")
	}

	_, err = resolveSpace(spaces, "nonexistent")
	if err == nil {
		t.Fatalf("expected unknown space error, got nil")
	}
}
