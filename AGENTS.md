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
├── cmd/
│   ├── root.go
│   ├── init.go
│   ├── login.go
│   ├── serve.go
│   ├── status.go
│   ├── sync.go
│   ├── install.go
│   ├── diff.go
│   ├── memory.go
│   ├── rules.go
│   └── skills.go
├── internal/
│   ├── config/
│   ├── cell/
│   ├── adapter/
│   ├── memory/
│   ├── perception/
│   ├── server/
│   └── sync/
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
- Each adapter is a pure function: (rules + skills + machine) -> agent config
