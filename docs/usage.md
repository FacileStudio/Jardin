# Mycelium — Usage

Every `mycelium` command and flag, with a realistic example each. Mycelium ships one binary that
is both the CLI you run on each machine and the server you self-host.

## Setup

### `mycelium init`

Scaffolds `~/.mycelium` — `memory/` with its `bugs/`, `tools/`, `projects/`, `conventions/`
and `syntheses/` subdirectories, plus `rules/`, `skills/`, `machines/` and `sessions/` — and
seeds `overview.md`, `index.md` and `log.md` when they do not exist. Safe to re-run.

```sh
mycelium init
```

### `mycelium login <url>`

Authenticates this machine against a Mycelium server and writes `url`, `token` and `machine`
into `~/.mycelium.yml`. By default it uses device authorization: it prints a user code, opens
your browser at `/authorize`, and polls until an admin approves. On success it also installs
the background sync service.

```sh
mycelium login https://mycelium.facile.studio --machine lucy --space studio
printf '%s' "$TOKEN" | mycelium login https://mycelium.facile.studio --token-stdin
```

| Flag | What it does |
|---|---|
| `-m, --machine` | Machine name to register. Defaults to the configured machine, else the hostname |
| `--token` | Skip the browser and use a token minted from the dashboard |
| `--token-stdin` | Read that token from stdin |
| `--password` | Authenticate with the server password instead |
| `--password-stdin` | Read the password from stdin |
| `--no-browser` | Print the authorization URL instead of opening a browser |
| `--no-daemon` | Do not install the background sync service |
| `--space` | Select a space to sync right after login, by name or id |

### `mycelium status`

Prints the machine name, the sync URL, the selected space (`common` when unset), the rule
and skill counts, and this week's session total.

## Syncing

### `mycelium sync`

The normal path: a three-way reconcile against `~/.mycelium/.sync-base.json`. Local edits
push, remote edits pull, deletions propagate both ways, and an edit-versus-edit conflict
keeps a `<path>.conflict` backup rather than losing a version.

### `mycelium push` / `mycelium pull`

Force one direction. `push` overwrites the server with local state; `pull` overwrites local
state with the server's. Use them when you know which side is right.

```sh
mycelium sync
mycelium pull
```

### `mycelium spaces list` / `mycelium spaces use`

Spaces are separate trees on the server. A machine syncs exactly one of them, or the common
tree. Switching resets `.sync-base.json`, so the next reconcile does not read the tree
switch as a mass deletion.

```sh
mycelium spaces list
mycelium spaces use studio
mycelium spaces use --none
```

| Flag | What it does |
|---|---|
| `--none` | Clear the space and go back to syncing the common tree |

## Content

### `mycelium memory search <query>` / `mycelium memory index`

Case-insensitive substring search across every `.md` file under `memory/`, printed as
`path:line` followed by the matching line. `index` prints `index.md`, the router page.

### `mycelium rules list` / `mycelium rules edit <name>`

Rules are ordered policy files under `~/.mycelium/rules/`. `edit` opens one in `$EDITOR`,
creating it if it does not exist. Emission order is `rule_order` from `~/.mycelium.yml` first,
then everything else.

### `mycelium skills list` / `mycelium skills add <name>`

Skills are reusable agent capabilities under `~/.mycelium/skills/`. `add` scaffolds a new one
with frontmatter, and refuses rather than overwriting an existing skill.

```sh
mycelium memory search "traefik"
mycelium rules edit 00-core
mycelium skills add changelog
```

## Generating agent config

### `mycelium install [agent] | --all`

Runs the adapters: `(rules + skills + machine) -> agent config`. With no agent and no
`--all`, it prints help rather than guessing.

```sh
mycelium install claude
mycelium install --all
mycelium diff codex
```

| Agent | Writes |
|---|---|
| `claude` | `~/.claude/CLAUDE.md`, skills as `~/.claude/skills/<name>/SKILL.md`, plus a `SessionStart` hook in `~/.claude/settings.json` |
| `codex` | `~/.codex/AGENTS.md`, skills as `~/.codex/skills/<name>/SKILL.md` |
| `gemini` | `~/.gemini/GEMINI.md` |
| `hermes` | `~/SOUL.md` |
| `copilot` | `.github/copilot-instructions.md` |
| `cursor` | `.cursor/rules/<name>.mdc` |

### `mycelium diff <agent>`

Shows what `install` would change for that agent, without writing anything.

## Sessions

### `mycelium sessions`

Lists the 20 most recent sealed session blocks: end time, project, duration, machine and
agent, branch, and output tokens.

### `mycelium sessions live`

Shows what is running right now across every machine. Liveness is computed at read time — a
block is live when its last event is inside the 15-minute gap timeout and the machine's
heartbeat is recent.

### `mycelium sessions scan`

Reads new agent activity out of the transcripts and folds it into blocks. The daemon runs it
before every sync.

| Flag | What it does |
|---|---|
| `--all` | Rebuild this machine's history from the full transcripts, ignoring stored offsets |

### `mycelium stats`

Aggregates sealed blocks into a table.

```sh
mycelium sessions scan --all
mycelium stats --since 30d --by project
```

| Flag | Default | What it does |
|---|---|---|
| `--since` | `7d` | Window: `7d`, `30d`, `12h`, or `all` |
| `--by` | `project` | Group by `project`, `machine`, `agent`, `branch`, or `model` |

## Daemon

### `mycelium daemon install` / `uninstall` / `status`

Installs a launchd or systemd service that ticks every 60 seconds: `sessions scan`, then
`sync`, then — at most every 5 minutes, gated by a `.last-install` stamp — `install` for
each configured agent. The launcher path is resolved through a stable symlink on `PATH`
rather than the versioned binary, so a Homebrew upgrade does not silently orphan the
service.

## Server

### `mycelium serve`

Runs the sync API and serves the dashboard. See
[configuration.md](configuration.md) for the environment it reads and
[deployment.md](deployment.md) for how it is deployed.

```sh
mycelium daemon install
mycelium serve --port 9000 --data /srv/mycelium
```

| Flag | Default | What it does |
|---|---|---|
| `--port` | `$PORT`, else `8420` | Listen port. Only overrides the environment when passed |
| `--data` | `$DATA_DIR`, else `~/.mycelium` | Data directory |

## Maintenance

### `mycelium update`

Self-updates to the latest release. Aliased as `mycelium upgrade`. `--check` reports whether
an update is available without installing it.

```sh
mycelium update --check
```

Back to the [documentation index](README.md).
