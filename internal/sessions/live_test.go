package sessions

import (
	"testing"
	"time"
)

func TestLiveSnapshotAndLiveness(t *testing.T) {
	dir := t.TempDir()
	state := newScanState()
	fold(state, "lucy", []Event{
		ev(0, "Jardin", 100),
		ev(time.Minute, "Jardin", 50),
		ev(2*time.Minute, "Casier", 10),
	}, t0.Add(3*time.Minute))

	now := t0.Add(3 * time.Minute)
	if err := writeLive(dir, "lucy", state, now); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadLive(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 open blocks, got %d", len(entries))
	}
	for _, e := range entries {
		if !e.Live || !e.MachineOnline || e.Machine != "lucy" {
			t.Fatalf("recent block must be live: %+v", e)
		}
	}

	later := now.Add(20 * time.Minute)
	if err := writeLive(dir, "lucy", state, later); err != nil {
		t.Fatal(err)
	}
	for _, e := range ReadLiveAt(t, dir, later) {
		if !e.MachineOnline {
			t.Fatalf("machine still ticking must read online: %+v", e)
		}
		if e.Live {
			t.Fatalf("block idle past the gap timeout must not be live: %+v", e)
		}
	}

	for _, e := range ReadLiveAt(t, dir, later.Add(StaleAfter+time.Second)) {
		if e.Live || e.MachineOnline {
			t.Fatalf("stale heartbeat must mark machine offline: %+v", e)
		}
	}
}

func ReadLiveAt(t *testing.T, dir string, at time.Time) []LiveEntry {
	t.Helper()
	entries, err := ReadLive(dir, at)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestLiveSkipsEmptyBlocksAndMissingMachines(t *testing.T) {
	dir := t.TempDir()
	state := newScanState()
	state.Open["ghost|claude"] = &Block{Project: "ghost", Agent: "claude", StartedAt: t0, EndedAt: t0}
	if err := writeLive(dir, "lucy", state, t0); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadLive(dir, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("zero-event block must not be published, got %+v", entries)
	}

	if entries, err = ReadLive(t.TempDir(), t0); err != nil || len(entries) != 0 {
		t.Fatalf("missing sessions dir must yield empty, got %v %v", entries, err)
	}
}

func TestLiveFileIsNotReadAsBlocks(t *testing.T) {
	dir := t.TempDir()
	state := newScanState()
	fold(state, "lucy", []Event{ev(0, "Jardin", 10)}, t0)
	if err := writeLive(dir, "lucy", state, t0); err != nil {
		t.Fatal(err)
	}
	blocks, err := ReadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 0 {
		t.Fatalf("live.json must not be parsed as sealed blocks, got %d", len(blocks))
	}
}

func TestReadBlocksDedupesById(t *testing.T) {
	dir := t.TempDir()
	block := finalize(&Block{Project: "Jardin", Machine: "lucy", Agent: "claude", StartedAt: t0, EndedAt: t0.Add(10 * time.Minute)})
	if err := appendBlocks(dir, "lucy", []Block{block, block}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBlocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("duplicate ids must collapse to one block, got %d", len(got))
	}
}
