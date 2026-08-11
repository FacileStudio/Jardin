# Mycelium

One brain for all your AI coding agents, synced across every machine.

Mycelium (French for *garden*) keeps a single canonical store of agent memory, rules, and
skills, generates the native config each agent expects, and syncs the whole garden over
HTTP. Teach one agent something on one machine, and every other agent on every other
machine knows it too.

Live at [mycelium.facile.studio](https://mycelium.facile.studio).

```
   rules ─┐
  skills ─┼─▶  mycelium install  ─▶  CLAUDE.md · ~/.codex/AGENTS.md · GEMINI.md · …
 machine ─┘       + pi extension  ─▶  ~/.pi/agent/ (auto-discovered)
                  + canonical events ─▶  ~/.mycelium/events/<agent>/ (JSONL)
                         (one source of truth, many native configs)

  memory  ◀──▶  mycelium sync  ◀──▶  Mycelium server  ◀──▶  every other machine
```

Every coding agent reinvents the same context: your conventions, the bug you fixed last
week, the gotcha in that one deploy script. Mycelium stores that once, as plain markdown,
and fans it out. The agents stay thin; the brain is shared.

## What it does

- Stores memory, rules, and skills as plain markdown under `~/.mycelium`, with no database
- Generates native config for `claude`, `codex`, `gemini`, `cursor`, `copilot`, `hermes`
  and ships a pi extension that integrates Pi into the same session tracking
- Syncs every machine against a Mycelium server with a three-way reconcile that never drops
  a version
- Runs a background daemon that scans agent activity, syncs, and refreshes configs
- Tracks Claude Code and Pi sessions into sealed time blocks, with live presence
  across machines, via a canonical JSONL event store at `~/.mycelium/events/<agent>/`
- Reports Claude subscription limits per machine, straight from Claude Code, no credential
  needed
- Serves a SvelteKit dashboard for browsing and editing the whole brain from a browser, with
  charts on the Memory, Machines and Sessions pages
- Authorizes new machines from the browser, with per-machine scoped tokens hashed at rest
- Publishes sealed sessions to the Antenne as `agent_session.created` events, and subscription
  windows crossing their threshold as `usage_alert.created`

## Stack

| Layer | Tech |
|---|---|
| CLI | Go 1.26, cobra, YAML config at `~/.mycelium.yml` |
| API | Go 1.26, chi, [tronc](https://github.com/FacileStudio/tronc) 0.8.0, no database |
| Client | SvelteKit 2, Svelte 5 (runes), Tailwind CSS 4, `adapter-static` |
| Storage | Plain markdown files on disk |
| Deploy | Docker Compose, single distroless container behind Traefik |

## Quick start

```sh
curl -fsSL https://raw.githubusercontent.com/FacileStudio/Mycelium/main/install.sh | bash
```

Installs to `~/.local/bin` via [facile](https://github.com/FacileStudio/facile), the suite
installer. Pass `--bin-dir <dir>` to change that, `--source` to build from source.

Already have `facile`:

```sh
facile install mycelium
```

```sh
brew install FacileStudio/tap/mycelium
```

Then set up this machine:

```sh
mycelium init                                # scaffold ~/.mycelium
mycelium login https://mycelium.facile.studio  # opens a browser to authorize this machine
mycelium sync                                # pull the shared brain
mycelium install --all                       # generate config for every agent
mycelium daemon install                      # optional: background sync
```

Open the dashboard and copy the master prompt from **Settings → Connect your agents** into
each agent so it knows how to read, write, and sync the shared brain.

### Pi (coding agent)

Pi auto-discovers extensions from `~/.pi/agent/extensions/`. The `mycelium` extension
bridges Pi into Mycelium's session tracking:

```sh
# The extension lives at ~/.pi/agent/extensions/mycelium/index.ts
# It is auto-discovered on next pi start (no install needed)

# Mycelium's canonical event store collects events from every agent:
ls ~/.mycelium/events/pi/   # monthly JSONL shards
mycelium sessions scan       # folds events into sealed blocks
mycelium sessions            # show sessions from all agents
```

The extension hooks into pi's `message_end`, `before_agent_start`, and
`resources_discover` events to log token usage, inject session context, and
expose Mycelium skills. A live status bar shows session cost and tokens.

### Self-hosting the server

```sh
cp .env.example .env
docker compose up -d
```

One container serves the API and the dashboard on port `8420`. In `APP_ENV=production` the
server refuses to start without `PASSWORD` or `OIDC_ISSUER`, rather than quietly serving
every request as admin.

### Local development

```sh
mise run hooks
go build -o mycelium .
mise run check
```

## Configuration

The CLI reads `~/.mycelium.yml`, written by `mycelium login`. The server reads its
configuration from the environment, once, at startup.

| Variable | What it does |
|---|---|
| `APP_ENV` | `development`, `staging`, or `production` |
| `PASSWORD` | Shared password for `mycelium login --password` |
| `DATA_DIR` | Where the markdown tree lives, default `~/.mycelium` |
| `OIDC_ISSUER` | Authentik issuer URL; setting it makes the client credentials required |
| `SSO_ONLY` | Disables password login, requires `OIDC_ISSUER` |

Full reference: [docs/configuration.md](docs/configuration.md).

## Structure

```
main.go      Entry point — tronc healthcheck, then the cobra tree
cmd/         One file per command (init, login, sync, install, serve, sessions, spaces, usage)
internal/
  adapter/   One file per agent (claude, codex, gemini, cursor, copilot, hermes)
  cell/      Local store: scaffold and read the markdown tree
  config/    ~/.mycelium.yml and the data directory paths
  daemon/    Background sync service (launchd / systemd)
  env/       Server configuration, loaded and validated once at startup
  memory/    Memory search and index
  server/    Sync API, dashboard backend, spaces, OIDC, Antenne emitter
  sessions/  Transcript scanning, sessionization, canonical event store, shards, stats, live presence
  sync/      HTTP client: three-way reconcile by checksum
  usage/     Claude subscription limits: status-line ingest, OAuth cross-check
apps/client/ SvelteKit dashboard, served by the Go binary
docs/        Architecture, configuration, usage, development, deployment, API
```

## Documentation

| Doc | What's in it |
|---|---|
| [Architecture](docs/architecture.md) | Request flow, the data tree, adapters, sync, spaces |
| [Configuration](docs/configuration.md) | Every environment variable and default |
| [Usage](docs/usage.md) | Every CLI command and flag |
| [Development](docs/development.md) | Local setup, tests, the quality gate |
| [Deployment](docs/deployment.md) | Docker Compose, Dokploy, Traefik routing, releases |
| [API](docs/api.md) | HTTP endpoints and payloads |

---

Part of the [Facile Suite](https://facile.studio) — self-hosted tools for creative studios
and freelancers. One login, zero cloud dependency.
