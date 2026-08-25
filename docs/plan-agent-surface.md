# Plan: the agent surface, and making sync's failure visible

Cold-start handoff. Written 2026-08-25. A fresh session needs nothing but this file.

## Goal

Move mycelium's agent surface from prose instructions to tools the model can see, and make a
stopped sync announce itself, so the manual sync step can leave the rules entirely.

## Why (evidence)

Two measurements, both from 2026-08-25.

`internal/daemon/daemon.go:20` sets `IntervalSeconds = 60`. `cmd/doctor.go:262` sets
`syncStaleAfter = 24 * time.Hour`. The daemon runs every minute and the health check complains
after a day, a gap of 1440x. Sync on ruche had been failing for roughly twenty hours and
`mycelium doctor` printed a tick beside `last sync: 19h43m36s ago`. Nothing surfaced it: not the
session recap, not the health check, only reading `journalctl` by hand.

Separately, the operating loop is prose in `~/.mycelium/rules/20-memory.md` that agents skim. The
session that produced this plan ran the start gate, received `401 Unauthorized`, and carried on
regardless. The instruction looked like a gate and was not one.

## Approach

Six tracks, four of them independent. Reuse what is already here rather than adding shapes: the
adapter registry (`Adapter` is `Name`/`Generate(Input)`/`TargetPaths`, and `Output.Files` is
path to content, so an MCP declaration is one more map entry and needs no interface change),
`internal/memory` and `internal/flow` called in process rather than shelled out, the
`--token-stdin` flag pattern from `cmd/login.go`, and `modelcontextprotocol/go-sdk v1.7.0`,
which is current as of 2026-07-27 and already pinned by nacelle.

Checked against: mycelium-agent-surface, agent-access-facile-apps, facile-cli-standard,
agents-md-standard, facile-test-layout. No migrations (mycelium has no database), no muse work,
no porte surface.

## Decisions already taken

Do not relitigate these. Each was settled with the user on 2026-08-25.

- **MCP is stdio, not a route.** The 2026-07-28 spec says a server "intending ... to be run
  locally SHOULD ... use the stdio transport to limit access to just the MCP client". Two of the
  three tools cannot be remote at all: `run_flow` executes `sh -c` on the calling machine, and
  the trust pin lives per machine in `~/.mycelium/.flow-trust.json`. The server already exposes
  `/api/flows` and `/api/flows/{name}` and pointedly no run endpoint. This is the documented
  exception to "MCP belongs in the API as a route", which stands for the other suite apps.
- **Track E is cut.** A filesystem watcher needs a long-lived process; `daemon.Run()` is oneshot
  under `Type=oneshot` on a 60s timer. Converting it would add a hang that `Restart=` cannot see,
  to a system that just lost twenty hours to a silent failure, in exchange for 59 seconds of
  latency on a wiki. If latency ever matters, change `OnUnitActiveSec` and nothing else.
- **The manual sync leaves the rules in both directions**, start-gate pull and post-write push,
  but only after Track D lands.
- **`claim` offline** takes the claim locally and reports it as unverified.
- **`memory add`** takes `--body`, with `--body-stdin` for prose.

## Steps

Order: D, C, A, B, G, F. D is cheap and fixes the failure class that actually cost time.

### Track D — make a stopped sync announce itself (no dependencies)

1. `cmd/doctor.go` — derive `syncStaleAfter` from `daemon.IntervalSeconds` when the daemon is
   installed, keeping a long threshold when it is not. A threshold is a multiple of the expected
   cadence, never a round human number. `[filet]`
2. `cmd/recap.go` — emit one staleness line from `recap --hook`, **only when stale**. Silence
   when healthy is the point; a line every session is a line nobody reads.
3. `internal/daemon/systemd.go` — add `OnFailure=` to the generated unit.
   `internal/daemon/service.go` — the launchd equivalent.
4. `cmd/doctor_test.go` — a test that the check goes red at the derived threshold. Mutation-check
   it: neuter the comparison and confirm the test fails.

**Exit:** stop the daemon, wait past the threshold, and both `doctor` and the next session's
recap say so without being asked.

### Track C — the write command (no dependencies)

5. `internal/memory/write.go` — new. Append a finding to a page, bump `updated:` in the
   frontmatter, add the one-line pointer to `index.md`, append the `log.md` line, run the
   English-only check, all atomically.
6. `cmd/memory_add.go` — new, alongside `memory.go` and `memory_ratify.go`. `--body` plus
   `--body-stdin`, following `cmd/login.go`'s `--token-stdin`. The log description is an
   **argument, never derived from the diff**: per `mycelium-agent-surface`, a log line records
   what was wrong before and what still holds, which a diff does not contain. No git verb on the
   surface. `[cli-standard]` `[agent-surface]`
7. Sync inline after the write, best effort. A sync failure must never fail the write.
   `[agent-surface rule 2]`
8. `internal/memory/write_test.go` — one runnable check that all four bookkeeping steps happen,
   and one that a failing sync still leaves the finding on disk.

**Exit:** one `mycelium memory add` produces a finding, an index pointer and a log line, and
still succeeds with the network unplugged.

### Track A — the MCP server (no dependencies)

9. `go.mod` — add `github.com/modelcontextprotocol/go-sdk v1.7.0`. `[module-path]`
10. `internal/mcpserver/server.go` — new. `mcp.NewServer` with three tools, calling
    `internal/memory` and `internal/flow` directly. Never shell out to the binary.
11. `internal/mcpserver/tools.go` — `search_memory` and `list_flows` get `ReadOnlyHint: true`;
    `run_flow` gets `DestructiveHint: true` and `OpenWorldHint: true`. Every tool carries an
    `outputSchema` and returns `structuredContent`, so `list_flows` reports trust as a field
    rather than a column of text. Annotations are UX only: clients must treat them as untrusted,
    so the flow trust pin remains the actual gate.
12. `internal/mcpserver/errors.go` — an untrusted flow returns a tool **execution** error
    (`isError: true`) naming `mycelium flow trust <name>`, which clients pass to the model for
    self-correction, rather than a protocol error.
13. `cmd/mcp.go` — new. `mycelium mcp`, `mcp.StdioTransport`. `[cli-standard]`
14. `internal/mcpserver/server_test.go` — assert `tools/list` returns three tools, each with
    explicit annotations and a valid schema, in deterministic order.

**Exit:** `mycelium mcp` answers `tools/list` with three annotated tools, and `run_flow` against
an untrusted flow returns an actionable execution error rather than an exit code.

### Track B — one door per agent (needs A's command name only)

15. `internal/adapter/claude.go`, `codex.go`, `opencode.go`, `gemini.go` — add the agent's
    mcpServers file to `Output.Files`. Existing target paths for reference:
    `~/.codex/`, `~/.config/opencode/`, and Claude's own. `[agents-md-standard]`
16. `internal/adapter/adapter.go` — add a capability marker to `Input` so a rule can render for
    a tool-capable agent or a CLI-only one.
17. `~/.mycelium/rules/20-memory.md`, `30-flows.md` — fence the CLI-versus-tool sentences with a
    comment marker, reusing the `<!-- lang:fr -->` precedent already in this corpus. An agent is
    told to call `search_memory` **or** to run `mycelium memory search`, never both.
18. `~/.mycelium/rules/20-memory.md` — remove the manual sync, both the start gate and the
    post-write step. **Only after D has landed.**
19. `~/.mycelium/memory/conventions/mycelium-agent-surface.md` — mark the "sync as an explicit
    act" clause `[SUPERSEDED by: nacelle sync-gate hook, 2026-08-23]`. Leave the rest: the
    domain-versus-storage line and the five rules are still right.

**Exit:** `mycelium install claude` writes a working mcpServers entry, and the generated rules
mention exactly one of tool or command. `mycelium diff <agent>` is clean on a second run.

### Track G — claims that mean something (no dependencies)

20. `cmd/claim.go` — take a claim against `GET /claims` when the server is reachable, falling
    back to `sessions.ReadClaimsLive(config.DataDir(), ...)` when it is not, and saying so in
    the output. Today it only ever reads local files, so two machines can both see no claims for
    up to a minute and collide.
21. `cmd/claim_test.go` — one check that an unreachable server still yields a claim, marked
    unverified.

**Exit:** claiming a repo already claimed on another machine within the last minute is refused
while online, and succeeds with an explicit "unverified" note while offline.

### Track F — nacelle stops naming a Facile tool (needs B installed and proven)

22. `~/Code/Facile/nacelle`: delete `tools/mycelium.go`, `tools/mycelium_test.go`, and the
    Mycelium switch across `tui/`, roughly 216 plus 22 lines. The SDK is a general Go agent
    library; the same argument is already recorded for antenne in `agent-access-facile-apps`:
    "a hard dependency in the SDK trades general usefulness for one integration".

**Exit:** nacelle greps clean for "mycelium", its gate passes, and the three tools still reach a
nacelle session through the `mcp:` list in `~/.nacelle.yml`.

## Files to modify or create

- `cmd/doctor.go`, `cmd/recap.go` — staleness, derived and surfaced
- `internal/daemon/systemd.go`, `internal/daemon/service.go` — failure notification
- `internal/memory/write.go` — new
- `cmd/memory_add.go` — new
- `internal/mcpserver/{server,tools,errors}.go` — new package
- `cmd/mcp.go` — new
- `internal/adapter/{adapter,claude,codex,opencode,gemini}.go` — MCP declaration, capability marker
- `cmd/claim.go` — server-authoritative when online
- `go.mod` — one dependency
- `~/.mycelium/rules/{20-memory,30-flows}.md` and the agent-surface convention — outside the repo
- nacelle: `tools/mycelium*.go` deleted, `tui/` switch removed

## Exit criteria

`sh scripts/check.sh` exits 0 and filet reports no new findings. `mycelium mcp` lists three
annotated tools. `mycelium memory add` does four bookkeeping steps in one call and survives an
offline sync. `doctor` and `recap` both go loud within minutes of a stopped daemon. `claim`
refuses a live claim from another machine. nacelle no longer mentions mycelium.

## Risks

- **Ordering is load-bearing.** Removing the sync instructions before Track D lands recreates the
  exact twenty-hour outage this plan exists to prevent. D first, always.
- **`mycelium install` merges additively.** It adds its entry beside an old one rather than
  replacing it, and `mycelium diff` reports "No changes" because the entry it wants is present.
  That produced two duplicate hooks during the rename. Track B must remove stale entries, not
  only write new ones.
- **Track F is irreversible in one direction.** Delete nacelle's tools before B works on the
  machine that uses it and a nacelle session loses flows and memory search entirely.
- **A push to main deploys production.** `git push origin main` rebuilds mycelium.facile.studio
  in about twenty seconds and does not wait for CI. The lefthook pre-push hook is the only gate.
- **Latency was never the problem.** If Track D alone removes the pain, stop and reassess before
  spending the rest.

## Skip

No remote or HTTP MCP transport: `/api/memory/search` already answers over HTTP for agents that
are not on these machines, and they must never reach `run_flow`. No `sync`, `doctor` or `install`
tools; things that should always happen are hooks, not decisions. No filesystem watcher, see
Track E above. No merging `log.md` into commits, settled in `mycelium-agent-surface`. No
shortening the daemon interval as a substitute for Track D.
