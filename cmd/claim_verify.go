package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/sessions"
)

// checkClaim asks the server who holds id's project and falls back to the
// local claim files when it cannot.
//
// The fallback is why verified exists. Local files arrive on a daemon tick, so
// two agents on two machines can both read "no claims" for up to a minute and
// both take the repo — the collision this check exists to stop. The server sees
// both immediately, so its answer is the one worth refusing on. But mycelium is
// local-first and a lock that fails closed would stop the work every time a
// laptop was on a train, so an unreachable server downgrades the verdict rather
// than ending the command. Every remote failure lands in err, which is a note
// to print and never a reason to return early.
func checkClaim(cfg *config.MyceliumConfig, id claimIdentity) claimVerdict {
	entries, err := fetchServerClaims(cfg)
	if err != nil {
		entries = sessions.ReadClaimsLive(config.DataDir(), id.project, time.Now())
	}
	return claimVerdict{holder: claimHolder(entries, id), verified: err == nil, err: err}
}

// fetchServerClaims reads every claim the server knows about, scoped to the
// configured space so a space member is answered about their own repos rather
// than the common root's.
func fetchServerClaims(cfg *config.MyceliumConfig) ([]sessions.ClaimEntry, error) {
	if cfg.ServerURL() == "" || cfg.AuthToken() == "" {
		return nil, errors.New("no server configured")
	}
	target := cfg.ServerURL() + "/api/claims"
	if space := cfg.SpaceID(); space != "" {
		target += "?space_id=" + url.QueryEscape(space)
	}
	ctx, cancel := context.WithTimeout(context.Background(), claimVerifyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claims: %s", resp.Status)
	}
	var entries []sessions.ClaimEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// claimHolder returns the claim on id's project that belongs to someone else,
// or nil when the repo is free.
//
// It blocks on MachineOnline rather than Live deliberately. Live also requires
// the claim to have been touched within sessions.StaleAfter, three minutes, and
// only 'claim note' touches one — so an agent that claimed a repo and then did
// the work would stop holding it after three quiet minutes. That is exactly the
// lock-that-is-not-a-lock this check replaces. A claim from a machine that
// stopped heartbeating does not block, and a holder that finished without
// releasing is cleared with 'mycelium claim done'.
func claimHolder(entries []sessions.ClaimEntry, id claimIdentity) *sessions.ClaimEntry {
	for i := range entries {
		e := &entries[i]
		if !strings.EqualFold(e.Project, id.project) {
			continue
		}
		if e.Machine == id.machine && e.Agent == id.agent {
			continue
		}
		if e.MachineOnline {
			return e
		}
	}
	return nil
}

// claimTaken renders the refusal: who holds the repo, since when, and the one
// command that takes it back. An agent that is told only "refused" retries.
func claimTaken(e *sessions.ClaimEntry) error {
	state := "idle"
	if e.Live {
		state = "active"
	}
	return fmt.Errorf("%s is claimed by %s/%s (%s since %s): %s\n  take it over with 'mycelium claim done -p %s -m %s --agent %s'",
		e.Project, e.Machine, e.Agent, state, sessions.FormatDuration(time.Since(e.StartedAt)), e.Task,
		e.Project, e.Machine, e.Agent)
}
