# Mycelium — Architecture

How the CLI, the server, and the markdown tree fit together, and what happens on a sync.

## Runtime topology

```
Internet ──▶ Traefik ──▶ mycelium serve (:8420) ──┬──▶ /health, /ready   liveness + readiness
                                                   ├──▶ /api/health      same, under the API
                                                   ├──▶ /api/*           handlers
                                                   └──▶ everything else  SvelteKit build
                                                                       │
                                                              $DATA_DIR (a volume)
                                                              markdown + JSON state
                                                                       │
                                                              Antenne (WebSocket, out)
```

There is no database. Every byte the server owns is a file under `DATA_DIR`: the markdown
tree it syncs, plus a handful of dot-prefixed JSON files for tokens, users, spaces,
settings, and the emit ledger.

On the machine side there is no server at all — the CLI reads and writes the same shape of
tree under `~/.mycelium` and talks to the server over HTTP.

## Components

| Package | Responsibility |
|---|---|
| `cmd/` | The cobra command tree, one file per command |
| `internal/config` | `~/.mycelium.yml` and the `DATA_DIR` paths |
| `internal/cell` | Scaffolds the local tree and reads rules, skills, and the machine block |
| `internal/adapter` | One pure function per agent: `(rules + skills + machine) -> files` |
| `internal/memory` | Substring search and `index.md` reads over the memory tree |
| `internal/sync` | HTTP client, three-way reconcile against a local base manifest |
| `internal/sessions` | Transcript scanning, sessionization, shards, stats, timelines, live presence |
| `internal/usage` | Subscription-limit snapshots: status-line ingest, OAuth cross-check, history |
| `internal/consolidate` | Episodic-to-semantic consolidation: episode readers, heuristic proposer, local durability judge, storage gate, dedupe, wiki writes |
| `internal/daemon` | launchd / systemd service that ticks scan, sync, and install |
| `internal/env` | Server configuration, read and validated once at startup |
| `internal/server` | Sync API, dashboard backend, spaces, OIDC, Antenne emitter |
| `apps/client` | SvelteKit dashboard, built to static files and served by the binary |

## The data tree

Both the machine (`~/.mycelium`) and the server (`DATA_DIR`) hold the same shape:

```
memory/          the brain — durable, non-obvious knowledge
  overview.md    always-read summary
  index.md       one-line-per-page router
  log.md         append-only history
  bugs/ tools/ projects/ conventions/ standards/ syntheses/ people/
rules/           ordered policy files
skills/          reusable agent skills
machines/        per-machine context blocks
sessions/        <machine>/<YYYY-MM>.jsonl sealed session blocks, plus live.json
usage/           <machine>/current.json latest snapshot, <YYYY-MM>.jsonl history
```

`mycelium init` creates the directories and seeds `overview.md`, `index.md`, and `log.md`.

Server-side state lives beside the tree and is never synced: `tokens.json`, `.users.json`,
`.spaces.json`, `.settings.json`, `.pool-ledger.json`. On a machine, `.sync-base.json`,
`.sessions-state.json`, `.flow-trust.json` and `.memory-ratified.json` are likewise local-only.

## Normative pages

A page carrying `type: standard` is normative: it says what a repository must do, rather than
recording what an agent observed. `~/.mycelium/.memory-ratified.json` records, per page, the
checksum a human accepted on this machine and the day they accepted it. `internal/memory/
ratify.go` compares the two and reports one of four standings — `ratified`, `not ratified`,
`CHANGED`, `MISSING`.

Three properties are load-bearing:

- **It gates authority, never availability.** Every page syncs, exists and ranks in whatever
  standing it is in. `CHANGED` marks a search result and fails `doctor`; it hides nothing.
- **Nothing is written into the page.** The state is a dotfile beside the tree, for the same
  reason conflict markers never land in a page.
- **The pin does not sync.** A pin made on one machine says nothing on another. If it
  travelled, one machine accepting a wrong edit would clear the flag on every machine, which
  is precisely the propagation the check exists to catch.

The pin is content-addressed rather than event-addressed, so `mycelium memory revert` back to an
accepted version restores `ratified` on its own.

## Request lifecycle

`httpx.NewRouter` from tronc builds the chi router with the suite middleware stack, then
`health.Mount` registers the probes at both the root and under `/api`. Every API route
lives inside a single `router.Route("/api", ...)` with its own `NotFound` and
`MethodNotAllowed` handlers, so an unknown API path answers a 404 error envelope instead of
falling through to the SPA and returning 200 plus HTML. The SPA handler is mounted last, on
`/*`, and only when `spa.Available` finds an `index.html` in `CLIENT_DIR`.

Failures cross the wire as `{"error":{"code","message"}}` through `tronc/httpjson` and
`tronc/errors`. Two statuses tronc's code-to-status map does not cover — 405 and 503 — are
written by a local `writeStatusError` so they stay themselves instead of collapsing into a
generic 500.

Path parameters are read through `pathParam`, never `chi.URLParam` directly. chi matches on
the raw path whenever the request carries percent-encoding and hands the parameter back
still encoded, where the `http.ServeMux` it replaced decoded it; without the unescape a
`%2F` would walk straight past the traversal guards and an `encodeURIComponent`'d member
email would never match a stored one.

## Authentication

Three token scopes, all stored as SHA-256 hashes in `tokens.json`, never in plaintext:

| Scope | Who holds it | Reaches |
|---|---|---|
| `admin` | Password session, and OIDC users flagged admin | Settings, tokens, devices, everything |
| `user` | Non-admin OIDC sessions | Spaces and their content |
| `sync` | Machines | Content and sync routes only |

Sessions carry an `expires_at` 30 days out and are deleted on first use after expiry;
machine tokens do not expire. Minting a token replaces any prior token with the same name,
so re-login rotates instead of accumulating.

Three ways in:

- **Password.** `POST /api/auth/login`, constant-time compared, rate-limited to 10 attempts
  per minute per client IP. A request with a machine name mints a `sync` token; without one
  it mints an `admin` browser session.
- **Device authorization.** `mycelium login <url>` calls `POST /api/auth/device/start`, prints
  a user code, and polls `POST /api/auth/device/poll` every 5 seconds. An admin approves it
  from `/authorize` in the dashboard. Codes live 10 minutes, at most 256 pending at a time,
  and a token is handed out exactly once — polling after approval consumes the request.
- **OIDC.** `GET /api/auth/oidc` sets a state cookie and redirects to the identity
  provider. The
  callback verifies the ID token, requires an `email` claim, upserts the user into
  `.users.json` (the first user ever seen becomes admin), mints a session, and hands it to
  the SPA in the URL fragment of the success URL. Provider discovery is lazy, so the server
  still boots when the IdP is unreachable.

When neither `PASSWORD` nor `OIDC_ISSUER` is set, every request is served as `admin` and the
server logs a warning at startup. `APP_ENV=production` refuses to start in that state.

## Spaces

The tree directly under `DATA_DIR` is the **common** scope: the instance owner's private
data, not a shared bucket. Admins reach it, machine tokens reach it, and every other
signed-in user is denied until an admin adds them to a space.

Each space lives at `spaces/<uuid>/` with its own `memory/`, `rules/`, `skills/` and
`sessions/` subtrees, and membership roles `owner`, `admin`, `member` in `.spaces.json`.

A caller-supplied `space_id` is untrusted input. Every content and sync route resolves its
root through `scopeRoot`, which rejects a `space_id` containing `/`, `\` or `.` before it is
ever used as a path component and funnels the rest through the `spaceAccess` membership
guard. The common tree fences `spaces/` out of the file sync entirely, so space content is
unreachable without membership.

## Sync

`mycelium sync` is a three-way reconcile against `~/.mycelium/.sync-base.json`, the manifest of
what both sides agreed on last time. Local edits push, remote edits pull, deletions
propagate both ways, and a genuine edit-versus-edit conflict keeps the losing version at
`~/.mycelium/.conflicts/<path>` rather than dropping it. Conflict markers are never written
into a page. `mycelium push` and `mycelium pull` force one direction.

The wire format is checksums and file bodies: `GET /api/sync/tree` returns a path,
SHA-256, size and mtime per file; `GET`, `PUT` and `DELETE /api/sync/files/*` move the
bodies. Server state is fenced out of the tree walk — `tokens.json`, anything starting with
a dot, `.conflict` backups, and the `spaces/` subtree.

Switching spaces resets `.sync-base.json`, otherwise the reconcile would read the tree
switch as a mass deletion.

## Adapters

An adapter is a pure function from `(rules + skills + machine)` to a set of files. Each one
self-registers in `internal/adapter`, so adding an agent is one small file.

| Agent | Writes |
|---|---|
| `agents` | `~/.agents/AGENTS.md`, skills as `~/.agents/skills/<name>/SKILL.md` |
| `claude` | `~/.claude/CLAUDE.md`, skills as `~/.claude/skills/<name>/SKILL.md` |
| `codex` | `~/.codex/AGENTS.md`, skills as `~/.codex/skills/<name>/SKILL.md` |
| `opencode` | `~/.config/opencode/AGENTS.md`, skills as `~/.config/opencode/skills/<name>/SKILL.md` |
| `gemini` | `~/.gemini/GEMINI.md`, skills as `~/.gemini/skills/<name>/SKILL.md` |
| `hermes` | `~/SOUL.md` |
| `copilot` | `.github/copilot-instructions.md` |
| `cursor` | `.cursor/rules/<name>.mdc` |

`agents` is the only adapter not named for a tool. It targets the AGENTS.md specification's
own global-base path, which is stewarded cross-vendor rather than by any one vendor, so a
single tree serves every agent that follows the convention — nacelle reads both paths today
— instead of needing one more adapter per tool. Both `~/.agents/AGENTS.md` and
`~/.agents/skills/**/SKILL.md` are named by the specification itself, and Gemini CLI already
reads `~/.agents/skills/` as an alias for its own — a second consumer of the same tree.

It is also the only adapter whose output opens with a generated-by notice. The others write
inside a directory their own vendor owns, so authorship is unambiguous; `~/.agents` is where
the specification tells a user to put their *own* global instructions, which makes it the one
target where mycelium can plausibly overwrite something handwritten. Run `mycelium diff agents`
before the first install on a machine that may already have one.

**Known risk**: an open proposal would move the global base to `$XDG_CONFIG_HOME/agents/
AGENTS.md`. It is unresolved, and consumers read `~/.agents` today, so that is what this
writes — but the path is not settled the way a vendor directory is.

The `claude` adapter also merges two things into `~/.claude/settings.json`: a `SessionStart`
hook that injects `mycelium recap` output as agent context, and a `statusLine` running
`mycelium usage --statusline`. Both merges are additive — unknown keys, existing hooks and a
`statusLine` the user configured themselves all survive, and an install with nothing left to
add writes nothing.

## Session tracking

`internal/sessions` tails Claude Code transcripts under `~/.claude/projects/*/*.jsonl`,
keeping per-file byte offsets in `~/.mycelium/.sessions-state.json`. User and assistant lines
are heartbeats; token usage is deduped by `requestId`, because a streamed response repeats
identical usage lines.

Sessionization is gap-based: a gap larger than `GapTimeout` (15 minutes) starts a new block,
and a block idle longer than `SealTimeout` (30 minutes) is sealed and appended to
`sessions/<machine>/<YYYY-MM>.jsonl`. Shards are append-only and single-writer per machine,
so they ride the normal file sync with no conflict risk. Block IDs are the first 16 hex
characters of `sha256(machine|agent|project|started_at)`, so a full rescan deduplicates
downstream.

Liveness is computed at read time, never stored — a sleeping machine would otherwise
advertise itself as working forever.

`sessions.Timeline` buckets the same sealed blocks over time for the dashboard's charts,
gap-filling every UTC day or month in range so a quiet week is a flat stretch rather than a
missing point. Series are ranked by total active seconds and capped at `MaxSeries` (6): the top
five plus a folded `Other`, because muse's `chartColor` wraps past six and a seventh series
would reuse the first colour.

## Usage limits

`internal/usage` tracks how much of each Claude subscription window is spent, per machine. Two
rules shape the whole package.

**It needs no credential.** Claude Code hands its status-line command a JSON payload on stdin
carrying a `rate_limits` object — `used_percentage` and `resets_at` as epoch seconds, per
window — for subscribers. `mycelium usage --statusline` parses it, records it, and prints the
one-line status. That payload only appears after the session's first API response, so a
brand-new session shows nothing for a few seconds and `/api/usage` legitimately answers `[]`
until the hook has run once. An OAuth token from `claude setup-token` enables `--live` as an
optional exact cross-check against Anthropic's usage endpoint; a standard `sk-ant-api…` API key
cannot read subscription limits at all and is rejected with a pointer to `setup-token`.

**Freshness is computed at read time, never stored** — the same rule liveness follows, for the
same reason. A recorded percentage is only a claim about a moment: it stops being true the
instant its window resets, and a stored freshness flag would keep reporting 68% for hours after
the window rolled over. So `expired`, `resets_in_seconds`, `age_seconds` and `stale` (older
than `StaleAfter`, 15 minutes) are all derived against the current clock on every read.
`used_percentage` is never rewritten: an expired window still reports what was last observed,
and `expired` is what tells the client not to present it as current. Stored samples are never
back-filled or mutated, so the history shards stay an append-only record of what was observed
when. `Record` keeps the newer of the stored and incoming snapshot, so the OAuth path answering
from its 5-minute cache cannot overwrite a fresher status-line reading.

Storage mirrors `sessions/`: `usage/<machine>/current.json`, written atomically through a
temp file and a rename, and `usage/<machine>/<YYYY-MM>.jsonl`, append-only and single-writer
per machine, so both ride the normal file sync with no conflict risk. The status line runs on
nearly every keystroke, so a history line lands only when a window moved at least
`HistoryDelta` (1 point), the window set changed, or the last sample aged past
`HistoryThrottle` (5 minutes). `usage/.oauth-cache.json` is dot-prefixed and therefore fenced
out of the sync.

The daemon runs `usage --live` on each tick, but only on machines where a token resolves —
without one the numbers already arrive from the status line, and the endpoint rate-limits hard
enough that polling it unasked would be rude.

### Threshold alerts on the Antenne

When `usage_alerts` is on, a window crossing `usage_threshold` is published to the Antenne as a
`usage_alert.created` event. The emitter is the **server's** (`mycelium serve`), not the machine's:
the server learns about usage through the normal file sync, so an alert lands within a sync tick.
That is the intended latency — this is a "you are running out of runway" nudge, not a realtime
tripwire.

**Mycelium publishes an event and nothing else.** No email, no push, no webhook. Antenne is the
alert aggregator and owns delivery; enabling this in Settings does not by itself notify anyone.

**It is edge-triggered per window instance, not per tick.** A bare `used_percentage >= threshold`
check would re-emit forever. `resets_at` is what uniquely identifies a window *instance*, so the
dedupe key is

```
sha256("usage_alert|" + email + "|" + window + "|" + resets_at + "|" + threshold)
```

truncated to 16 hex like `sessions.Block.ID`, and stored in the shared `.pool-ledger.json` under a
`usage:` prefix so it can never collide with a block ID. `mycelium_usage_alert_created_` + those 16
hex is the envelope's `idempotency_key` — a prefixed derivative in the same house style as
`sessions.Block.IdempotencyKey()`, not the raw key. So a crash between emit and ledger write yields
a duplicate the downstream absorbs rather than a lost alert. When the window rolls over Anthropic
returns a new `resets_at`, the identity changes, and the next crossing legitimately alerts again.
Including the threshold in the key means lowering the setting re-arms the alert, which is what a
user changing it expects.

**The key is emailed, not machined**, because a subscription limit belongs to an Anthropic account
and the resolved email is that account's identity here. Two machines on the same plan observe the
same window with the same `resets_at`, so keying on `machine` fired two alerts for one crossing —
one event per laptop, for one limit. The key rests on one load-bearing property: `resets_at` is an
absolute instant handed back by Anthropic, not a computed now-plus-remaining, so every machine on
the account observes it identically and no quantization is needed to make the keys match.

Two machines mapped to *different* emails still alert separately, which is correct — those are two
people to tell.

Eligible snapshots are therefore grouped by that identity and each group emits **one** alert
carrying the **highest** `used_percentage` in it, with `machine` set to the snapshot that supplied
that maximum and ties broken by the lexicographically smallest machine name. The readings all
describe one shared account, so the maximum is the closest thing to the truth — a threshold
decision should not rest on a lower stale reading when a higher one is available. `machine` is
consequently metadata about where the winning reading came from, never part of the identity.

Five conditions suppress an otherwise-crossing window:

| Suppressed when | Why |
|---|---|
| The window is expired (`resets_at` in the past) | It has already rolled over; alerting would report history as news |
| `resets_at` is unknown | No per-instance identity, so the alert would repeat every tick |
| The snapshot's `updated_at` predates `emit_since` | The same no-surprise-backfill watermark the session emitter uses |
| No email resolves for the machine | The event contract keys on `user_email`, exactly as `pendingBlocks` drops unattributable session blocks |
| The key is already in the ledger | It already went out |

The email skip deliberately writes **no** ledger entry, so the window instance stays eligible and
alerts once an email is configured rather than being permanently consumed. Resolution is the
session emitter's: the per-machine override first, then the global `user_email`.

Every window is evaluated, not just `five_hour` — a weekly limit at 80% matters more, not less.
The threshold boundary is inclusive. A *stale* snapshot is still eligible: the crossing genuinely
happened, and staleness only means nobody has reported since.

Alerts are computed across the common tree and every space tree. A machine present in more than
one tree still yields a single alert per window instance, because the dedupe key carries no scope.

`enveloppe` defines no usage object, and `ObjectType` and `Action` are typed strings on a generic
`Event[T]`, so Mycelium emits a contract-shaped event using an object constant defined locally in
`internal/server`. Adopting the type into `enveloppe` is a deliberate follow-up decision, not a
side effect of this change — `enveloppe` is a cross-repo contract consumed by Opus and Sablier.
The pool client announces what it emits, so `usage_alert.created` is declared in the `Emit` list
alongside `agent_session.created`; an undeclared type may be dropped. Both payloads are specified
in [Published events](api.md#published-events).

## Consolidation

`internal/consolidate` is the missing link between episodic capture and semantic memory:
the daemon reads recent episodes from `events/<agent>/*.jsonl` and consolidates them into
`memory/` on its own — no user interaction, no cloud tokens.

The pipeline runs once per daemon tick at most, rate-limited to one run per hour regardless
of tick frequency:

1. **Source.** A shapeless reader keys on directory, not schema: unknown JSONL yields
   episodes with text best-effort extracted from any `"message"`, `"content"` or `"text"`
   value. Per-harness adapters come later if a second harness's events demand it.
2. **Heuristic proposer.** Deterministic patterns over episode text propose candidates in
   three kinds: error→fix pairs (error followed within a few lines by resolution), explicit
   gotcha markers ("gotcha", "turns out", "the fix was", "note that"), and repeated failures
   across sessions (same error at two or more distinct timestamps).
3. **Durability judge.** Each heuristic hit goes to a small local Ollama model
   (`consolidate.judge_model`) with one yes/no question: will this be useful in 30 days?
   The judge **fails open** — Ollama unreachable or unconfigured returns accept with a
   `heuristic-fallback` verdict, so an offline machine keeps consolidating on heuristics
   alone. The gate still applies downstream either way.
4. **Storage gate.** The four rules from the storage-gate policy, executable: changes future
   behavior, non-obvious (concrete anchors like paths, dates, error strings), grounded
   (episode refs as provenance), and no secrets — name heuristics shared with `internal/`
   flow plus token-shape patterns. Rejections are typed with a rule name and reason, never
   just a bool, so the daemon log and doctor can say why something was dropped.
5. **Dedupe.** Each surviving candidate is embedded and searched against the existing wiki
   (lexical fallback when embeddings are off). Three outcomes: `NOOP`, `CREATE`, `SUPERSEDE`.
   SUPERSEDE requires all three of high embedding similarity, judge agreement that the two
   texts actually contradict, and the candidate's claim being dated newer than the struck
   claim's `**Date**:` — and unlike the judge it **fails closed**: when any leg cannot be
   verified, the answer is NOOP. Auto-striking a correct claim on weak evidence is the one
   mistake this stage must not make silently.
6. **Write.** CREATE appends a `### finding` block with `**Date**:`/`**Source**:` lines to
   the matched page, or a new page under the matching top-level dir. SUPERSEDE strikes the
   old claim in place — `~~struck~~ [SUPERSEDED by: ...]` — and writes the correction
   beneath it. Nothing is ever deleted; supersede-in-place is the forgetting mechanism.
7. **Cursor.** `.consolidate-cursor.json` records, per source, the last-processed timestamp
   plus the hash of that exact line, advanced only after the write phase succeeds. A crash
   mid-run reprocesses nothing on the next run; deleting the file is the documented escape
   hatch for a full replay.

Every model call is localhost Ollama; there are zero network calls outside the machine, and
heuristic-fallback mode works fully offline.

## Cross-app integration

- **Journal.** When `JOURNAL_URL` and `JOURNAL_TOKEN` are both set, the tronc logger is
  wrapped with the Journal SDK handler and structured logs ship to Journal.
- **Antenne.** When enabled in Settings, an emitter loop publishes sealed session blocks
  to the Antenne as `agent_session.created` events on the
  [enveloppe](https://github.com/FacileStudio/enveloppe) contract, every 30 seconds or on
  demand. Blocks shorter than a minute are skipped, as are blocks with no resolvable
  `user_email` — the event contract keys on it. The shards are the durable outbox and
  `.pool-ledger.json` records what already went out; because block IDs are deterministic, a
  crash between emit and ledger write yields a duplicate that Sablier's idempotency ledger
  absorbs rather than a lost or double-counted entry. On first enable the `emit_since`
  watermark defaults to now, so there is no surprise backfill. The same emitter, ledger and
  settings block also publish `usage_alert.created` when a subscription window crosses its
  configured threshold — see [Threshold alerts on the Antenne](#threshold-alerts-on-the-antenne).
- **Registre.** SSO federates to registre at `sso.facile.studio` over standard OIDC.
  Mycelium is registre's first consumer: it authenticates as the `mycelium`
  application client, and the redirect it is registered under is exact-match, so
  `OIDC_REDIRECT_URL` has to keep the `/api` prefix this server mounts its API on.

Back to the [documentation index](README.md).
