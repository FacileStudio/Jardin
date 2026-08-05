# Mycelium

**One brain for all your AI coding agents, synced across every machine.**

Mycelium (French for *garden*) keeps a single canonical store of agent **memory**,
**rules**, and **skills**, then generates the native config each agent expects —
Claude Code's `CLAUDE.md`, Codex's `AGENTS.md`, Gemini, Cursor, Copilot, Hermes —
and syncs the whole garden over HTTP. Teach one agent something on one machine, and
every other agent on every other machine knows it too.

```
   rules ─┐
  skills ─┼─▶  mycelium install  ─▶  CLAUDE.md · ~/.codex/AGENTS.md · GEMINI.md · …
 machine ─┘                         (one source of truth, many native configs)

  memory  ◀──▶  mycelium sync  ◀──▶  Mycelium server  ◀──▶  every other machine
```

## Why

Every coding agent reinvents the same context: your conventions, the bug you fixed
last week, the gotcha in that one deploy script. Mycelium stores that **once**, as plain
markdown, and fans it out. The agents stay thin; the brain is shared.

- **Portable** — rules and skills are written once and adapted to each agent's format.
- **Persistent** — a tiered, MemGPT-style wiki (`overview` → `index` → topic pages)
  that agents read before acting and write back to after.
- **Synced** — markdown over HTTP, one Bearer token per machine. Background daemon
  keeps every machine in sync every 5 minutes.
- **Plain files** — no database, no lock-in. It's just markdown in `~/.mycelium`.

## Install

```bash
brew install FacileStudio/tap/mycelium
# or
go install github.com/FacileStudio/Mycelium@latest
```

## Quickstart

```bash
mycelium init                              # scaffold ~/.mycelium (memory, rules, skills, machines)
mycelium login https://mycelium.facile.studio # opens your browser to authorize this machine
mycelium sync                              # pull the shared brain
mycelium install --all                     # generate config for every agent (or: mycelium install claude)
mycelium daemon install                    # optional: background sync every 5 min
```

Then open the dashboard (**Settings → Connect your agents**) and paste the master
prompt into each agent so it knows how to read, write, and sync the shared brain.

## Commands

| Command | Does |
| --- | --- |
| `mycelium init` | Scaffold the `~/.mycelium` data directory |
| `mycelium login <url>` | Authenticate with a Mycelium server, save sync config |
| `mycelium sync` / `push` / `pull` | Sync memory, rules, and skills with the server |
| `mycelium status` | Show machine, sync state, and content summary |
| `mycelium memory search <query>` | Substring search across all memory (`path:line`) |
| `mycelium memory index` | Print `index.md`, the memory router |
| `mycelium rules list` / `edit <name>` | Manage shared rules (`~/.mycelium/rules/`) |
| `mycelium skills list` / `add <name>` | Manage shared skills (`~/.mycelium/skills/`) |
| `mycelium install [agent] \| --all` | Generate agent config from rules + skills + machine |
| `mycelium diff <agent>` | Preview what `install` would change |
| `mycelium daemon install` / `uninstall` / `status` | Manage the background sync service |
| `mycelium update` (alias `upgrade`) | Self-update to the latest release |
| `mycelium serve` | Run the sync server + dashboard API (self-host) |

Agents: `claude`, `codex`, `gemini`, `cursor`, `copilot`, `hermes`.

## How it works

```
~/.mycelium/
├── memory/          # the brain — durable, non-obvious knowledge
│   ├── overview.md  #   always-read summary (core memory)
│   ├── index.md     #   one-line-per-page router
│   ├── log.md       #   append-only history
│   └── bugs/ tools/ projects/ conventions/ syntheses/
├── rules/           # ordered policy files (00-…, 10-…, 20-…)
├── skills/          # reusable agent skills
└── machines/        # per-machine context blocks
```

- **Memory** is a tiered wiki modeled on MemGPT/Letta: a compact always-in-context
  overview, a scannable index that routes to topic pages, and archival pages
  retrieved on demand. Agents read `overview → index → 1-3 pages`, never the whole
  thing.
- **Adapters** are pure functions: `(rules + skills + machine) → agent config`.
  Each one writes the format its agent expects — `claude` → `~/.claude/CLAUDE.md`
  with skills as commands; `codex` → `~/.codex/AGENTS.md` with skills as
  `~/.codex/skills/<name>/SKILL.md`; and so on. Adding an agent is one small file
  in `internal/adapter/`.
- **Sync** is plain markdown over HTTP with a per-machine Bearer token (tokens are
  hashed at rest, scoped, and rate-limited on the server). It's a three-way reconcile
  against a local base manifest: your edits push, others' edits pull, deletions
  propagate, and a true edit-vs-edit conflict keeps a `.conflict` backup instead of
  losing a version. `mycelium push` / `pull` force one direction when you want it.

## Self-hosting

The server bundles the sync API and the dashboard:

```bash
docker compose up -d        # one container: sync API + dashboard, port 8420
# or run the binary directly:
mycelium serve
```

Copy `.env.example` to `.env` first — in `APP_ENV=production` the server refuses to start
without a `PASSWORD` or an SSO issuer, rather than quietly serving every request as admin.

The dashboard (`apps/client`, SvelteKit) lets you browse and edit memory, rules, and
skills, manage machines and sync tokens, authorize new devices, and copy the master
prompt — the whole brain, from the browser.

## Development

See [AGENTS.md](AGENTS.md) for the tech stack, project layout, and conventions.

```bash
go build -o mycelium .
go test ./...
```

Releases are tag-triggered via GoReleaser + GitHub Actions, published to the
`FacileStudio/homebrew-tap` Homebrew tap.
