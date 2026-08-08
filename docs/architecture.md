# Jardin — Architecture

How the CLI, the server, and the markdown tree fit together, and what happens on a sync.

## Runtime topology

```
Internet ──▶ Traefik ──▶ jardin serve (:8420) ──┬──▶ /health, /ready   liveness + readiness
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
tree under `~/.jardin` and talks to the server over HTTP.

## Components

| Package | Responsibility |
|---|---|
| `cmd/` | The cobra command tree, one file per command |
| `internal/config` | `~/.jardin.yml` and the `DATA_DIR` paths |
| `internal/cell` | Scaffolds the local tree and reads rules, skills, and the machine block |
| `internal/adapter` | One pure function per agent: `(rules + skills + machine) -> files` |
| `internal/memory` | Substring search and `index.md` reads over the memory tree |
| `internal/sync` | HTTP client, three-way reconcile against a local base manifest |
| `internal/sessions` | Transcript scanning, sessionization, shards, stats, timelines, live presence |
| `internal/usage` | Subscription-limit snapshots: status-line ingest, OAuth cross-check, history |
| `internal/daemon` | launchd / systemd service that ticks scan, sync, and install |
| `internal/env` | Server configuration, read and validated once at startup |
| `internal/server` | Sync API, dashboard backend, spaces, OIDC, Antenne emitter |
| `apps/client` | SvelteKit dashboard, built to static files and served by the binary |

## The data tree

Both the machine (`~/.jardin`) and the server (`DATA_DIR`) hold the same shape:

```
memory/          the brain — durable, non-obvious knowledge
  overview.md    always-read summary
  index.md       one-line-per-page router
  log.md         append-only history
  bugs/ tools/ projects/ conventions/ syntheses/
rules/           ordered policy files
skills/          reusable agent skills
machines/        per-machine context blocks
sessions/        <machine>/<YYYY-MM>.jsonl sealed session blocks, plus live.json
usage/           <machine>/current.json latest snapshot, <YYYY-MM>.jsonl history
```

`jardin init` creates the directories and seeds `overview.md`, `index.md`, and `log.md`.

Server-side state lives beside the tree and is never synced: `tokens.json`, `.users.json`,
`.spaces.json`, `.settings.json`, `.pool-ledger.json`. On a machine, `.sync-base.json` and
`.sessions-state.json` are likewise local-only.

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
- **Device authorization.** `jardin login <url>` calls `POST /api/auth/device/start`, prints
  a user code, and polls `POST /api/auth/device/poll` every 5 seconds. An admin approves it
  from `/authorize` in the dashboard. Codes live 10 minutes, at most 256 pending at a time,
  and a token is handed out exactly once — polling after approval consumes the request.
- **OIDC.** `GET /api/auth/oidc` sets a state cookie and redirects to Authentik. The
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

`jardin sync` is a three-way reconcile against `~/.jardin/.sync-base.json`, the manifest of
what both sides agreed on last time. Local edits push, remote edits pull, deletions
propagate both ways, and a genuine edit-versus-edit conflict keeps a `<path>.conflict`
backup rather than losing a version. `jardin push` and `jardin pull` force one direction.

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
| `claude` | `~/.claude/CLAUDE.md`, skills as `~/.claude/skills/<name>/SKILL.md` |
| `codex` | `~/.codex/AGENTS.md`, skills as `~/.codex/skills/<name>/SKILL.md` |
| `gemini` | `~/.gemini/GEMINI.md` |
| `hermes` | `~/SOUL.md` |
| `copilot` | `.github/copilot-instructions.md` |
| `cursor` | `.cursor/rules/<name>.mdc` |

The `claude` adapter also merges two things into `~/.claude/settings.json`: a `SessionStart`
hook that injects `jardin recap` output as agent context, and a `statusLine` running
`jardin usage --statusline`. Both merges are additive — unknown keys, existing hooks and a
`statusLine` the user configured themselves all survive, and an install with nothing left to
add writes nothing.

## Session tracking

`internal/sessions` tails Claude Code transcripts under `~/.claude/projects/*/*.jsonl`,
keeping per-file byte offsets in `~/.jardin/.sessions-state.json`. User and assistant lines
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
window — for subscribers. `jardin usage --statusline` parses it, records it, and prints the
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
  watermark defaults to now, so there is no surprise backfill.
- **Porte.** SSO federates to Authentik at `porte.facile.studio` over standard OIDC.

Back to the [documentation index](README.md).
