package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/sessions"
)

// claimFixture isolates one claim command from the machine running the test:
// its own data dir, its own home so no real ~/.mycelium.yml is read, and its
// own identity. Nothing here can reach the live server.
func claimFixture(t *testing.T, serverURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.URLEnv, serverURL)
	t.Setenv(config.TokenEnv, "tok")
	claimProject, claimMachine, claimAgent, claimBranch, claimBody = "Mycelium", "lucy", "claude", "main", ""
	t.Cleanup(func() {
		claimProject, claimMachine, claimAgent, claimBranch, claimBody = "", "", "", "", ""
	})
	return dir
}

// claimsServer answers GET /api/claims with entries, and fails the test on any
// other request: a claim check must not touch a route it was not given.
func claimsServer(t *testing.T, entries []sessions.ClaimEntry) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/claims" {
			t.Errorf("requested %s, want /api/claims", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want the configured bearer", got)
		}
		json.NewEncoder(w).Encode(entries)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func heldEntry(machine, agent string, online bool) sessions.ClaimEntry {
	now := time.Now()
	return sessions.ClaimEntry{
		Claim: sessions.Claim{
			Project: "Mycelium", Machine: machine, Agent: agent,
			Task: "Track G", StartedAt: now.Add(-10 * time.Minute), UpdatedAt: now,
		},
		Live:          online,
		MachineOnline: online,
	}
}

// idleEntry is a claim whose machine still heartbeats but whose last note is
// older than sessions.StaleAfter, so ReadClaimsLive calls it idle rather than
// live. An agent doing the work it claimed looks exactly like this.
func idleEntry(machine, agent string) sessions.ClaimEntry {
	e := heldEntry(machine, agent, true)
	e.UpdatedAt = time.Now().Add(-2 * sessions.StaleAfter)
	e.Live = false
	return e
}

// The decided policy: an unreachable server never blocks the work. The claim is
// taken from the local view and the verdict says it was not verified.
func TestClaimStartTakesTheClaimWhenTheServerIsUnreachable(t *testing.T) {
	dir := claimFixture(t, "http://127.0.0.1:1")

	if err := claimStartCmd.RunE(claimStartCmd, []string{"track g"}); err != nil {
		t.Fatalf("claim start refused while offline: %v", err)
	}
	saved, err := sessions.LoadClaim(dir, "Mycelium", "lucy", "claude")
	if err != nil || saved == nil {
		t.Fatalf("LoadClaim = %v, %v, want the claim written locally", saved, err)
	}
	if saved.Task != "track g" {
		t.Errorf("saved task = %q, want %q", saved.Task, "track g")
	}

	verdict := checkClaim(&config.MyceliumConfig{}, claimIdentity{machine: "lucy", agent: "claude", project: "Mycelium"})
	if verdict.verified {
		t.Errorf("verdict.verified = true, want the offline claim marked unverified")
	}
	if verdict.err == nil {
		t.Errorf("verdict.err = nil, want the reason the server did not answer")
	}
}

// Offline plus a conflicting local claim is still not a refusal: the local view
// is a warning, never a lock, or a stale file could strand an agent for good.
func TestClaimStartStillTakesTheClaimWhenTheLocalViewShowsAnotherHolder(t *testing.T) {
	dir := claimFixture(t, "http://127.0.0.1:1")
	held := heldEntry("ruche", "opus", true).Claim
	if err := sessions.SaveClaim(dir, &held); err != nil {
		t.Fatal(err)
	}

	if err := claimStartCmd.RunE(claimStartCmd, []string{"track g"}); err != nil {
		t.Fatalf("claim start refused on local evidence while offline: %v", err)
	}
	if saved, _ := sessions.LoadClaim(dir, "Mycelium", "lucy", "claude"); saved == nil {
		t.Fatal("no claim was written, want the offline claim to land anyway")
	}
}

// Online, the server is the authority: it sees the other machine's claim inside
// the sync interval that the local files are still behind on.
func TestClaimStartRefusesAClaimTheServerReportsHeldElsewhere(t *testing.T) {
	srv := claimsServer(t, []sessions.ClaimEntry{heldEntry("ruche", "opus", true)})
	dir := claimFixture(t, srv.URL)

	err := claimStartCmd.RunE(claimStartCmd, []string{"track g"})
	if err == nil {
		t.Fatal("claim start succeeded, want a refusal naming the holder")
	}
	for _, want := range []string{"ruche/opus", "Track G", "mycelium claim done"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err.Error(), want)
		}
	}
	if saved, _ := sessions.LoadClaim(dir, "Mycelium", "lucy", "claude"); saved != nil {
		t.Error("a claim was written despite the refusal")
	}
}

// A server that reports no live holder must not stand in the way, and the
// verdict is verified so nothing is printed about sync.
func TestClaimStartSucceedsWhenTheServerReportsNoHolder(t *testing.T) {
	srv := claimsServer(t, []sessions.ClaimEntry{})
	dir := claimFixture(t, srv.URL)

	if err := claimStartCmd.RunE(claimStartCmd, []string{"track g"}); err != nil {
		t.Fatalf("claim start: %v", err)
	}
	if saved, _ := sessions.LoadClaim(dir, "Mycelium", "lucy", "claude"); saved == nil {
		t.Fatal("no claim was written")
	}
	cfg := &config.MyceliumConfig{}
	if verdict := checkClaim(cfg, claimIdentity{machine: "lucy", agent: "claude", project: "Mycelium"}); !verdict.verified {
		t.Errorf("verdict.verified = false against a reachable server (%v)", verdict.err)
	}
}

// Who blocks and who does not: an offline machine's claim is dead weight, our
// own claim is a re-claim, and the project match ignores case.
func TestClaimHolderBlocksOnlyAnotherOnlineMachine(t *testing.T) {
	id := claimIdentity{machine: "lucy", agent: "claude", project: "mycelium"}
	cases := []struct {
		name    string
		entries []sessions.ClaimEntry
		want    string
	}{
		{"another online machine holds it", []sessions.ClaimEntry{heldEntry("ruche", "opus", true)}, "ruche"},
		{"the holder's machine is offline", []sessions.ClaimEntry{heldEntry("ruche", "opus", false)}, ""},
		{"the holder is online but has not noted in a while", []sessions.ClaimEntry{idleEntry("ruche", "opus")}, "ruche"},
		{"the claim is our own", []sessions.ClaimEntry{heldEntry("lucy", "claude", true)}, ""},
		{"a second agent on our machine holds it", []sessions.ClaimEntry{heldEntry("lucy", "codex", true)}, "lucy"},
		{"nothing is claimed", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			holder := claimHolder(tc.entries, id)
			if tc.want == "" {
				if holder != nil {
					t.Fatalf("claimHolder = %s/%s, want nil", holder.Machine, holder.Agent)
				}
				return
			}
			if holder == nil || holder.Machine != tc.want {
				t.Fatalf("claimHolder = %v, want a claim held by %s", holder, tc.want)
			}
		})
	}
}

// A claim on another repo is not this repo's problem, whatever its state.
func TestClaimHolderIgnoresOtherProjects(t *testing.T) {
	other := heldEntry("ruche", "opus", true)
	other.Project = "Vision"
	if holder := claimHolder([]sessions.ClaimEntry{other}, claimIdentity{machine: "lucy", agent: "claude", project: "Mycelium"}); holder != nil {
		t.Fatalf("claimHolder = %s on %s, want nil", holder.Machine, holder.Project)
	}
}

// A server that answers with anything but 200 is an unreachable server: a 401
// from an expired token must degrade exactly like a dead socket, not refuse.
func TestCheckClaimTreatsAnUnauthorizedServerAsUnverified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	claimFixture(t, srv.URL)

	verdict := checkClaim(&config.MyceliumConfig{}, claimIdentity{machine: "lucy", agent: "claude", project: "Mycelium"})
	if verdict.verified {
		t.Fatal("verdict.verified = true on a 401, want it demoted to unverified")
	}
	if !strings.Contains(verdict.err.Error(), "401") {
		t.Errorf("verdict.err = %v, want the status in it", verdict.err)
	}
}

// A space member must be answered about their space's repos. Without the
// scope the server serves the common tree and the check reads the wrong list.
// The value is one that would split into two parameters unescaped.
func TestFetchServerClaimsCarriesTheConfiguredSpace(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("space_id")
		json.NewEncoder(w).Encode([]sessions.ClaimEntry{})
	}))
	defer srv.Close()
	t.Setenv(config.URLEnv, srv.URL)
	t.Setenv(config.TokenEnv, "tok")

	if _, err := fetchServerClaims(&config.MyceliumConfig{Space: "9f2a&b"}); err != nil {
		t.Fatalf("fetchServerClaims: %v", err)
	}
	if got != "9f2a&b" {
		t.Errorf("space_id = %q, want the configured space", got)
	}
}
