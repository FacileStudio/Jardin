# Ruche

Shared agent memory — manage wiki, rules, and skills across AI coding agents and machines.

## Tech Stack

- **Language**: Go 1.24+
- **CLI framework**: cobra (spf13/cobra)
- **Config**: TOML (BurntSushi/toml)
- **Release**: GoReleaser + GitHub Actions (tag-triggered), Homebrew tap via FacileStudio/homebrew-tap
- **Dependencies**: fatih/color (terminal colors)

## Key Commands

```bash
go build -o ruche .
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
│   ├── config/         # ~/.ruche.yml + paths
│   ├── cell/           # local store: read rules/skills/machine, scaffold
│   ├── adapter/        # one file per agent (claude, codex, gemini, cursor, copilot, hermes)
│   ├── memory/         # memory search + index
│   ├── daemon/         # background sync service (launchd/systemd)
│   ├── server/         # sync API + dashboard backend
│   └── sync/           # HTTP client: push/pull by checksum
├── apps/client/       # SvelteKit dashboard
├── Dockerfile
├── docker-compose.yml
├── .goreleaser.yml
└── .github/workflows/release.yml
```

## Conventions

- No inline comments in code
- Client config is YAML at `~/.ruche.yml`; data lives under `~/.ruche` (or `$DATA_DIR`)
- Storage is plain markdown files synced over HTTP to a Ruche server; auth is a Bearer token per machine, obtained via `ruche login <url>`
- Each adapter is a pure function `(rules + skills + machine) -> agent config`, self-registers via `init()` in `internal/adapter/`, and writes the format its agent expects
- Sync is a three-way reconcile against a local base manifest (`~/.ruche/.sync-base.json`): local edits push, remote edits pull, deletes propagate both ways, and a genuine edit-vs-edit conflict keeps a `<path>.conflict` backup (never silent loss). `ruche push`/`pull` force one direction
- The copy-paste master prompt shown in the dashboard lives in `apps/client/src/lib/agentPrompt.ts`
