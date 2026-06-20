# Ruche

Shared agent brain — manage wiki, rules, and skills across AI coding agents and machines.

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
│   ├── clone.go
│   ├── use.go
│   ├── status.go
│   ├── cells.go
│   ├── sync.go
│   ├── install.go
│   ├── diff.go
│   ├── brain.go
│   ├── rules.go
│   ├── skills.go
│   ├── perception.go
│   └── upgrade.go
├── internal/
│   ├── config/
│   ├── cell/
│   ├── git/
│   ├── adapter/
│   ├── brain/
│   ├── merge/
│   └── perception/
├── .goreleaser.yml
└── .github/workflows/release.yml
```

## Conventions

- No inline comments in code
- TOML for config files (ruche.toml, cell.toml)
- All storage is plain markdown files in git repos
- Each adapter is a pure function: (rules + skills + machine) -> agent config
