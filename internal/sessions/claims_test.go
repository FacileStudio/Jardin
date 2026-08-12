package sessions

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func writeLiveSnapshot(t *testing.T, dataDir, machine string, updatedAt time.Time) {
	t.Helper()
	if err := os.MkdirAll(machineDir(dataDir, machine), 0o755); err != nil {
		t.Fatal(err)
	}
	snap := LiveSnapshot{Machine: machine, UpdatedAt: updatedAt.UTC(), Open: []LiveBlock{}}
	data, _ := json.Marshal(snap)
	if err := os.WriteFile(livePath(dataDir, machine), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaimSaveLoadRelease(t *testing.T) {
	dir := t.TempDir()
	c := &Claim{
		Project: "Mycelium", Machine: "lucy", Agent: "pi", Branch: "main",
		Task: "fix sync", StartedAt: time.Now(), UpdatedAt: time.Now(), Body: "a\nb",
	}
	if err := SaveClaim(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadClaim(dir, "Mycelium", "lucy", "pi")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Task != "fix sync" || got.Body != "a\nb" {
		t.Fatalf("round-trip failed: %+v", got)
	}
	if err := ReleaseClaim(dir, "Mycelium", "lucy", "pi"); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadClaim(dir, "Mycelium", "lucy", "pi"); got != nil {
		t.Fatal("claim not released")
	}
}

func TestClaimPerAgentPathsDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	for _, tc := range []struct {
		machine, agent string
	}{
		{"lucy", "pi"},
		{"ruche", "pi"},
		{"lucy", "claude"},
	} {
		SaveClaim(dir, &Claim{
			Project: "Mycelium", Machine: tc.machine, Agent: tc.agent,
			Task: tc.machine + "/" + tc.agent, StartedAt: base, UpdatedAt: base,
		})
	}
	claims, err := ReadClaims(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 3 {
		t.Fatalf("expected 3 distinct claims, got %d", len(claims))
	}
	for _, c := range claims {
		if c.Project != "Mycelium" {
			t.Fatalf("wrong project on claim: %+v", c)
		}
	}
}

func TestReadClaimsLiveResolvesLiveness(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeLiveSnapshot(t, dir, "lucy", now.Add(-10*time.Second))
	writeLiveSnapshot(t, dir, "ruche", now.Add(-2*StaleAfter))

	SaveClaim(dir, &Claim{
		Project: "Mycelium", Machine: "lucy", Agent: "pi", Task: "fresh",
		StartedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-10 * time.Second),
	})
	SaveClaim(dir, &Claim{
		Project: "Mycelium", Machine: "lucy", Agent: "claude", Task: "idle",
		StartedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-4 * StaleAfter),
	})
	SaveClaim(dir, &Claim{
		Project: "Argent", Machine: "ruche", Agent: "pi", Task: "offline",
		StartedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-10 * time.Second),
	})

	entries := ReadClaimsLive(dir, "Mycelium", now)
	if len(entries) != 2 {
		t.Fatalf("expected 2 claims for Mycelium, got %d", len(entries))
	}
	byAgent := map[string]ClaimEntry{}
	for _, e := range entries {
		if !e.MachineOnline {
			t.Fatalf("lucy claim %s must be machine-online", e.Agent)
		}
		byAgent[e.Agent] = e
	}
	if !byAgent["pi"].Live {
		t.Error("recently touched claim should be live")
	}
	if byAgent["claude"].Live {
		t.Error("stale claim should not be live")
	}

	all := ReadClaimsLive(dir, "", now)
	if len(all) != 3 {
		t.Fatalf("--all should return every claim, got %d", len(all))
	}
}
