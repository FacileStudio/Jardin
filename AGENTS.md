# Mycelium

Shared agent memory — manage wiki, rules, and skills across AI coding agents and machines.

## Tech Stack

- **Language**: Go 1.26
- **CLI framework**: cobra (spf13/cobra)
- **Config**: YAML (`~/.mycelium.yml`) for the CLI, environment for the server (`internal/env`)
- **Server chassis**: [tronc](https://github.com/FacileStudio/tronc) — chi router with the
  suite's middleware stack, error envelope, structured logger, health probes, SPA handler
- **Release**: GoReleaser + GitHub Actions (tag-triggered), Homebrew tap via FacileStudio/homebrew-tap
- **Dependencies**: fatih/color (terminal colors), Journal SDK (log shipping)

## Key Commands

```bash
go build -o mycelium .
go run .
go install .
go test ./...
git tag v0.x.x && git push --tags
```

## Project Structure

```
.
├── main.go
├── cmd/                # cobra commands (one file per command)
│   ├── root.go init.go login.go status.go
│   ├── sync.go install.go diff.go daemon.go
│   └── memory.go rules.go skills.go serve.go
├── internal/
│   ├── config/         # ~/.mycelium.yml + paths
│   ├── cell/           # local store: read rules/skills/machine, scaffold
│   ├── adapter/        # one file per agent (claude, codex, gemini, cursor, copilot, hermes)
│   ├── memory/         # memory search + index
│   ├── sessions/       # agent session tracking: transcript scan, sessionize, shards, stats
│   ├── daemon/         # background sync service (launchd/systemd)
│   ├── server/         # sync API + dashboard backend + settings + Antenne emitter
│   ├── env/            # server configuration, loaded and validated once at startup
│   └── sync/           # HTTP client: push/pull by checksum
├── apps/client/       # SvelteKit dashboard (adapter-static, served by the Go binary)
├── Dockerfile          # single image: SPA build + Go build + distroless runtime
├── docker-compose.yml
├── .goreleaser.yml
└── .github/workflows/release.yml
```

## Server shape (mono-container)

- One binary, one container, one Traefik router `Host(mycelium.facile.studio)` — no
  `PathPrefix`, no `stripprefix`. The Go binary serves `/api/*` and, as the catch-all,
  the SvelteKit build from `$CLIENT_DIR`. Every API route lives under a single
  `router.Route("/api", ...)`, so an unknown API path answers a 404 envelope instead of
  falling through to the SPA and returning 200 + HTML
- Failures cross the wire as `{"error":{"code","message"}}` via `tronc/httpjson` +
  `tronc/errors`. `/health` and `/ready` answer at both the root and under `/api`
- Configuration is read once by `internal/env` and the process exits **1** when it is
  invalid: `SSO_ONLY` without `OIDC_ISSUER`, `OIDC_ISSUER` without client credentials, or
  `APP_ENV=production` with neither `PASSWORD` nor `OIDC_ISSUER` (which would serve every
  request as admin). See `.env.example`
- **The bus can be configured entirely from the environment.** `ANTENNE_URL`,
  `ANTENNE_SECRET` and `ANTENNE_USER_EMAIL` **override** `.settings.json` rather than
  seeding it, so a container is reproducible from its compose file and a stale settings
  file cannot outrank it; setting `ANTENNE_URL` alone turns emitting on. The dashboard
  renders the pinned fields read-only. Leave them unset to configure it from the UI.
  The settings payload's key is `antenne`; `nook` is still accepted on read for old
  files (`adoptLegacy`) and is **not** accepted on write
- chi hands path parameters back **percent-encoded** whenever the request carries any
  escaping, where the `http.ServeMux` it replaced decoded them. Read them with
  `pathParam(r, key)`, never `chi.URLParam` directly, or `%2F` walks straight past the
  traversal guards and an `encodeURIComponent`'d member email never matches a stored one

## Conventions

- No inline comments in code
- Client config is YAML at `~/.mycelium.yml`; data lives under `~/.mycelium` (or `$DATA_DIR`)
- Storage is plain markdown files synced over HTTP to a Mycelium server; auth is a Bearer token per machine, obtained via `mycelium login <url>`
- Each adapter is a pure function `(rules + skills + machine) -> agent config`, self-registers via `init()` in `internal/adapter/`, and writes the format its agent expects
- Sync is a three-way reconcile against a local base manifest (`~/.mycelium/.sync-base.json`): local edits push, remote edits pull, deletes propagate both ways, and a genuine edit-vs-edit conflict keeps a `<path>.conflict` backup (never silent loss). `mycelium push`/`pull` force one direction
- The copy-paste master prompt shown in the dashboard lives in `apps/client/src/lib/agentPrompt.ts`

## Spaces + SSO (v0.6)

- Suite-consistent spaces: the existing tree is the **Common** scope — the instance owner's private data, NOT a shared bucket (no migration). Admins and machine tokens reach it; every other signed-in user is denied and sees only spaces they belong to. This is deliberately Nuage's rule, not Sablier's (where `space_id IS NULL` has no owner filter, so any user reads everyone's 'personal' data); each space lives under `spaces/<uuid>/{memory,rules,skills,sessions}` server-side with membership roles owner/admin/member in `.spaces.json`. A caller-supplied `space_id` (query param on all content + sync routes) is untrusted input — every route funnels through the `spaceAccess` membership guard, and the common tree fences out `spaces/` so space content is unreachable without membership
- SSO via Authentik (porte): env-driven (`OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET/REDIRECT_URL/SUCCESS_URL`, `SSO_ONLY`) using go-oidc/v3; callback upserts the user by email (`.users.json`, first user becomes admin), mints a 30-day session token (name `session:<email>`, sha256 at rest), and hands it to the SPA via URL fragment on `/auth/callback`. Unset issuer = feature dormant, password login unchanged
- Token scopes: `admin` (settings/tokens/devices), `user` (OIDC non-admin sessions: spaces + content), `sync` (machines: content + sync only). Sessions expire; machine tokens don't
- CLI: `mycelium spaces list|use <name>`, `space:` in ~/.mycelium.yml, sync sends `space_id` — switching spaces resets `.sync-base.json` so the three-way merge doesn't read the tree switch as mass deletions

## Session tracking

- `internal/sessions` tails Claude Code transcripts (`~/.claude/projects/*/*.jsonl`) with per-file byte offsets kept in `~/.mycelium/.sessions-state.json` (never synced). Heartbeats are user/assistant lines; token usage is deduped by `requestId` (streamed responses repeat identical usage lines)
- Sessionization is gap-based (15 min gap joins, no padding, isolated heartbeat = 0 duration); a block idle >30 min is sealed and appended to `~/.mycelium/sessions/<machine>/<YYYY-MM>.jsonl`. Shards are append-only and single-writer per machine, so they ride the normal file sync with zero conflict risk
- Block IDs are deterministic (`sha256(machine|agent|project|start)`), so full rescans (`mycelium sessions scan --all`) and re-emits deduplicate downstream
- Live presence: each scan republishes `sessions/<machine>/live.json` (open blocks + heartbeat) which rides the normal sync, so every machine and the dashboard can see work in progress. Liveness is **computed at read time**, never stored — a sleeping machine would otherwise advertise itself as working forever. A block is live when its last event is inside the 15 min gap timeout AND the machine's heartbeat is inside `StaleAfter` (3 daemon ticks); `mycelium sessions live` and `GET /api/sessions/live` surface active / idle / machine-offline
- The daemon ticks every 60s so liveness stays fresh, but regenerating agent configs is write-heavy, so `install` is gated to every 5 min by the `.last-install` stamp
- The daemon runs `sessions scan` before each sync; `mycelium install claude` merges a SessionStart hook into `~/.claude/settings.json` that injects `mycelium recap` output as agent context
- The server (`mycelium serve`) can publish sealed blocks to the Antenne as `agent_session.created` (enveloppe contract) for Sablier to turn into time entries. Config lives in the server data dir as `.settings.json`, managed via `PUT /api/settings` (admin scope, dashboard Settings page); the emit ledger is `.pool-ledger.json`; on first enable the `emit_since` watermark defaults to now (no surprise backfill)
