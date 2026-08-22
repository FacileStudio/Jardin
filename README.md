# Jardin

One brain for all your AI coding agents, synced across every machine.

Jardin (French for *garden*) keeps a single canonical store of agent memory, rules, and
skills, generates the native config each agent expects, and syncs the whole garden over
HTTP. Teach one agent something on one machine, and every other agent on every other
machine knows it too.

Live at [jardin.facile.studio](https://jardin.facile.studio).

```
   rules ─┐
  skills ─┼─▶  jardin install  ─▶  CLAUDE.md · ~/.codex/AGENTS.md · GEMINI.md · …
 machine ─┘       + pi extension  ─▶  ~/.pi/agent/ (auto-discovered)
                  + canonical events ─▶  ~/.jardin/events/<agent>/ (JSONL)
                         (one source of truth, many native configs)

  memory  ◀──▶  jardin sync  ◀──▶  Jardin server  ◀──▶  every other machine
```

Every coding agent reinvents the same context: your conventions, the bug you fixed last
week, the gotcha in that one deploy script. Jardin stores that once, as plain markdown,
and fans it out. The agents stay thin; the brain is shared.

## What it does

- Stores memory, rules, and skills as plain markdown under `~/.jardin`, with no database
- Generates native config for `claude`, `codex`, `gemini`, `cursor`, `copilot`, `hermes`,
  `opencode`, and `agents` (the cross-vendor `~/.agents/` standard path)
  and ships a pi extension that integrates Pi into the same session tracking
- Syncs every machine against a Jardin server with a three-way reconcile that never drops
  a version
- Runs a background daemon that scans agent activity, syncs, and refreshes configs
- Tracks Claude Code and Pi sessions into sealed time blocks with estimated cost per model,
  live presence across machines, and a tabbed dashboard with charts and aggregation
- Reports subscription limits per machine, straight from Claude Code, no credential needed
- Serves a SvelteKit dashboard for browsing and editing the whole brain from a browser, with
  charts on the Memory, Machines and Sessions pages
- Authorizes new machines from the browser, with per-machine scoped tokens hashed at rest
- Publishes sealed sessions to the Antenne as `agent_session.created` events, and subscription
  windows crossing their threshold as `usage_alert.created`

## Stack

| Layer | Tech |
|---|---|
| CLI | Go 1.26, cobra, YAML config at `~/.jardin.yml` |
| API | Go 1.26, chi, [tronc](https://github.com/FacileStudio/tronc) 0.8.0, no database |
| Client | SvelteKit 2, Svelte 5 (runes), Tailwind CSS 4, `adapter-static` |
| Storage | Plain markdown files on disk |
| Deploy | Docker Compose, single distroless container behind Traefik |

## Quick start

```sh
curl -fsSL https://raw.githubusercontent.com/FacileStudio/Jardin/main/install.sh | bash
```

Installs to `~/.local/bin` via [facile](https://github.com/FacileStudio/facile), the suite
installer. Pass `--bin-dir <dir>` to change that, `--source` to build from source.

Already have `facile`:

```sh
facile install jardin
```

```sh
brew install FacileStudio/tap/jardin
```

Then set up this machine:

```sh
jardin init                                # scaffold ~/.jardin
jardin login https://jardin.facile.studio  # opens a browser to authorize this machine
jardin sync                                # pull the shared brain
jardin install --all                       # generate config for every agent
jardin daemon install                      # optional: background sync
```

Open the dashboard and copy the master prompt from **Settings → Connect your agents** into
each agent so it knows how to read, write, and sync the shared brain.

### Pi (coding agent)

Pi auto-discovers extensions from `~/.pi/agent/extensions/`. The `jardin` extension
bridges Pi into Jardin's session tracking:

```sh
# The extension lives at ~/.pi/agent/extensions/jardin/index.ts
# It is auto-discovered on next pi start (no install needed)

# Jardin's canonical event store collects events from every agent:
ls ~/.jardin/events/pi/   # monthly JSONL shards
jardin sessions scan       # folds events into sealed blocks
jardin sessions            # show sessions from all agents
```

The extension hooks into pi's `message_end`, `before_agent_start`, and
`resources_discover` events to log token usage, inject session context, and
expose Jardin skills. A live status bar shows session cost and tokens.

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
mise install
go build -o jardin .
mise run check
```

## Configuration

The CLI reads `~/.jardin.yml`, written by `jardin login`. The server reads its
configuration from the environment, once, at startup.

| Variable | What it does |
|---|---|
| `APP_ENV` | `development`, `staging`, or `production` |
| `PASSWORD` | Shared password for `jardin login --password` |
| `DATA_DIR` | Where the markdown tree lives, default `~/.jardin` |
| `OIDC_ISSUER` | Authentik issuer URL; setting it makes the client credentials required |
| `SSO_ONLY` | Disables password login, requires `OIDC_ISSUER` |

Full reference: [docs/configuration.md](docs/configuration.md).

## Structure

```
main.go      Entry point — tronc healthcheck, then the cobra tree
cmd/         One file per command (init, login, sync, install, serve, sessions, spaces, usage)
internal/
  adapter/   One file per agent (agents, claude, codex, gemini, cursor, copilot, hermes, opencode)
  cell/      Local store: scaffold and read the markdown tree
  config/    ~/.jardin.yml and the data directory paths
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
