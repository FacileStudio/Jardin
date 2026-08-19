package sessions

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

func ev(offset time.Duration, project string, out int64) Event {
	return Event{Time: t0.Add(offset), Agent: "claude", Project: project, TokensOut: out}
}

func TestFoldMergesWithinGap(t *testing.T) {
	state := newScanState()
	sealed := fold(state, "lucy", []Event{
		ev(0, "Mycelium", 100),
		ev(5*time.Minute, "Mycelium", 50),
		ev(14*time.Minute, "Mycelium", 25),
	}, t0.Add(20*time.Minute))

	if len(sealed) != 0 {
		t.Fatalf("expected no sealed blocks, got %d", len(sealed))
	}
	open := state.Open["mycelium|claude"]
	if open == nil {
		t.Fatal("expected open block")
	}
	if open.Events != 3 || open.TokensOut != 175 {
		t.Fatalf("bad accumulation: events=%d tokens=%d", open.Events, open.TokensOut)
	}
	if open.Duration() != 14*time.Minute {
		t.Fatalf("bad duration: %s", open.Duration())
	}
}

func TestFoldSealsOnGap(t *testing.T) {
	state := newScanState()
	sealed := fold(state, "lucy", []Event{
		ev(0, "Mycelium", 100),
		ev(16*time.Minute, "Mycelium", 50),
	}, t0.Add(17*time.Minute))

	if len(sealed) != 1 {
		t.Fatalf("expected 1 sealed block, got %d", len(sealed))
	}
	if sealed[0].Duration() != 0 {
		t.Fatalf("isolated heartbeat must have zero duration, got %s", sealed[0].Duration())
	}
	if sealed[0].ID == "" {
		t.Fatal("sealed block must carry an id")
	}
}

func TestFoldSealsStaleOpenBlocks(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{ev(0, "Mycelium", 100)}, t0.Add(1*time.Minute))
	sealed := fold(state, "lucy", nil, t0.Add(31*time.Minute))

	if len(sealed) != 1 {
		t.Fatalf("expected stale block sealed, got %d", len(sealed))
	}
	if len(state.Open) != 0 {
		t.Fatalf("expected no open blocks, got %d", len(state.Open))
	}
}

func TestFoldSeparatesProjects(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{
		ev(0, "Mycelium", 10),
		ev(time.Minute, "Sablier", 20),
	}, t0.Add(2*time.Minute))

	if len(state.Open) != 2 {
		t.Fatalf("expected 2 open blocks, got %d", len(state.Open))
	}
}

func TestFoldMergesCaseVariants(t *testing.T) {
	state := newScanState()
	fold(state, "lucy", []Event{
		{Time: t0, Agent: "claude", Project: "GFConseil", TokensOut: 10},
		{Time: t0.Add(time.Minute), Agent: "claude", Project: "gfconseil", TokensOut: 20},
	}, t0.Add(2*time.Minute))

	if len(state.Open) != 1 {
		t.Fatalf("case variants must merge into one block, got %d", len(state.Open))
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:FacileStudio/GFConseil.git": "GFConseil",
		"https://github.com/FacileStudio/Mycelium":    "Mycelium",
		"https://github.com/owner/repo.name.git":    "repo.name",
		"ssh://git@host.com/owner/Repo/":            "Repo",
		"":                                          "",
		"..":                                        "",
	}
	for in, want := range cases {
		if got := repoNameFromRemote(in); got != want {
			t.Fatalf("repoNameFromRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAggregateFoldsCase(t *testing.T) {
	blocks := []Block{
		{Project: "GFConseil", StartedAt: t0, EndedAt: t0.Add(time.Hour), TokensOut: 10},
		{Project: "gfconseil", StartedAt: t0, EndedAt: t0.Add(time.Hour), TokensOut: 20},
	}
	rows := Aggregate(blocks, time.Time{}, "project")
	if len(rows) != 1 {
		t.Fatalf("case variants must aggregate together, got %d rows", len(rows))
	}
	if rows[0].Key != "GFConseil" || rows[0].TokensOut != 30 {
		t.Fatalf("bad folded row: %+v", rows[0])
	}
}

func TestBlockIDDeterministic(t *testing.T) {
	a := Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0}
	b := Block{Project: "Mycelium", Machine: "lucy", Agent: "claude", StartedAt: t0}
	if a.computeID() != b.computeID() {
		t.Fatal("same natural key must yield same id")
	}
	c := Block{Project: "Mycelium", Machine: "ruche", Agent: "claude", StartedAt: t0}
	if a.computeID() == c.computeID() {
		t.Fatal("different machine must yield different id")
	}
}
