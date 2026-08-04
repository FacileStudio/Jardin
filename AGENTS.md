# Jardin

Shared agent memory — manage wiki, rules, and skills across AI coding agents and machines.

## Tech Stack

- **Language**: Go 1.24+
- **CLI framework**: cobra (spf13/cobra)
- **Config**: TOML (BurntSushi/toml)
- **Release**: GoReleaser + GitHub Actions (tag-triggered), Homebrew tap via FacileStudio/homebrew-tap
- **Dependencies**: fatih/color (terminal colors)

## Key Commands

```bash
go build -o jardin .
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
│   ├── config/         # ~/.jardin.yml + paths
│   ├── cell/           # local store: read rules/skills/machine, scaffold
│   ├── adapter/        # one file per agent (claude, codex, gemini, cursor, copilot, hermes)
│   ├── memory/         # memory search + index
│   ├── sessions/       # agent session tracking: transcript scan, sessionize, shards, stats
│   ├── daemon/         # background sync service (launchd/systemd)
│   ├── server/         # sync API + dashboard backend + settings + Nook pool emitter
│   └── sync/           # HTTP client: push/pull by checksum
├── apps/client/       # SvelteKit dashboard
├── Dockerfile
├── docker-compose.yml
├── .goreleaser.yml
└── .github/workflows/release.yml
```

## Conventions

- No inline comments in code
- Client config is YAML at `~/.jardin.yml`; data lives under `~/.jardin` (or `$DATA_DIR`)
- Storage is plain markdown files synced over HTTP to a Jardin server; auth is a Bearer token per machine, obtained via `jardin login <url>`
- Each adapter is a pure function `(rules + skills + machine) -> agent config`, self-registers via `init()` in `internal/adapter/`, and writes the format its agent expects
- Sync is a three-way reconcile against a local base manifest (`~/.jardin/.sync-base.json`): local edits push, remote edits pull, deletes propagate both ways, and a genuine edit-vs-edit conflict keeps a `<path>.conflict` backup (never silent loss). `jardin push`/`pull` force one direction
- The copy-paste master prompt shown in the dashboard lives in `apps/client/src/lib/agentPrompt.ts`

## Session tracking

- `internal/sessions` tails Claude Code transcripts (`~/.claude/projects/*/*.jsonl`) with per-file byte offsets kept in `~/.jardin/.sessions-state.json` (never synced). Heartbeats are user/assistant lines; token usage is deduped by `requestId` (streamed responses repeat identical usage lines)
- Sessionization is gap-based (15 min gap joins, no padding, isolated heartbeat = 0 duration); a block idle >30 min is sealed and appended to `~/.jardin/sessions/<machine>/<YYYY-MM>.jsonl`. Shards are append-only and single-writer per machine, so they ride the normal file sync with zero conflict risk
- Block IDs are deterministic (`sha256(machine|agent|project|start)`), so full rescans (`jardin sessions scan --all`) and re-emits deduplicate downstream
- The daemon runs `sessions scan` before each sync; `jardin install claude` merges a SessionStart hook into `~/.claude/settings.json` that injects `jardin recap` output as agent context
- The server (`jardin serve`) can publish sealed blocks to the Nook pool as `agent_session.created` (enveloppe contract) for Sablier to turn into time entries. Config lives in the server data dir as `.settings.json`, managed via `PUT /api/settings` (admin scope, dashboard Settings page); the emit ledger is `.pool-ledger.json`; on first enable the `emit_since` watermark defaults to now (no surprise backfill)
