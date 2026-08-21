package adapter

import (
	"slices"
	"strings"
	"testing"
)

// TestNamesAreSortedNotMapOrder guards the one property a map cannot give:
// Go randomises iteration per run, so before Names existed the help text, the
// "available" list in Get's error and the order install --all wrote its files
// all differed between two runs of the same binary. A test that only checked
// membership would pass on the broken version, so this asserts the ordering
// itself.
func TestNamesAreSortedNotMapOrder(t *testing.T) {
	names := Names()
	if len(names) < 2 {
		t.Fatalf("expected several registered adapters, got %v", names)
	}
	if !slices.IsSorted(names) {
		t.Fatalf("Names() = %v, want sorted", names)
	}
	if !slices.Contains(names, "agents") {
		t.Errorf("Names() = %v, missing \"agents\"", names)
	}
}

// TestAvailableIsTheSortedNamesJoined ties the display string to Names, so a
// future edit cannot make the help text and the error message disagree.
func TestAvailableIsTheSortedNamesJoined(t *testing.T) {
	if got, want := Available(), strings.Join(Names(), ", "); got != want {
		t.Fatalf("Available() = %q, want %q", got, want)
	}
}

// TestAllFollowsNamesAndHoldsNoNils is the other half of the ordering
// contract. All is what `install --all` iterates, so an order disagreeing
// with Names would put the two lists a user sees out of step — and a nil
// entry would panic on first use instead of reporting a missing adapter,
// which is the failure this signature exists to make unrepresentable.
func TestAllFollowsNamesAndHoldsNoNils(t *testing.T) {
	names := Names()
	all := All()
	if len(all) != len(names) {
		t.Fatalf("All() has %d adapters, Names() has %d", len(all), len(names))
	}
	for i, a := range all {
		if a == nil {
			t.Fatalf("All()[%d] is nil", i)
		}
		if a.Name() != names[i] {
			t.Errorf("All()[%d].Name() = %q, want %q", i, a.Name(), names[i])
		}
	}
}

// TestGetNamesTheAlternativesOnAMiss is why Available exists at all: a typo
// must answer with the list to pick from, not just a refusal.
func TestGetNamesTheAlternativesOnAMiss(t *testing.T) {
	_, err := Get("clade")
	if err == nil {
		t.Fatal("expected an error for an unknown adapter")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error must list the real adapters, got %q", err)
	}
}
