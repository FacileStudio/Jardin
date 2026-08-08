# Jardin — Usage

Every `jardin` command and flag, with a realistic example each. Jardin ships one binary that
is both the CLI you run on each machine and the server you self-host.

## Setup

### `jardin init`

Scaffolds `~/.jardin` — `memory/` with its `bugs/`, `tools/`, `projects/`, `conventions/`
and `syntheses/` subdirectories, plus `rules/`, `skills/`, `machines/` and `sessions/` — and
seeds `overview.md`, `index.md` and `log.md` when they do not exist. Safe to re-run.

```sh
jardin init
```

### `jardin login <url>`

Authenticates this machine against a Jardin server and writes `url`, `token` and `machine`
into `~/.jardin.yml`. By default it uses device authorization: it prints a user code, opens
your browser at `/authorize`, and polls until an admin approves. On success it also installs
the background sync service.

```sh
jardin login https://jardin.facile.studio --machine lucy --space studio
printf '%s' "$TOKEN" | jardin login https://jardin.facile.studio --token-stdin
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

### `jardin status`

Prints the machine name, the sync URL, the selected space (`common` when unset), the rule
and skill counts, and this week's session total.

## Syncing

### `jardin sync`

The normal path: a three-way reconcile against `~/.jardin/.sync-base.json`. Local edits
push, remote edits pull, deletions propagate both ways, and an edit-versus-edit conflict
keeps a `<path>.conflict` backup rather than losing a version.

### `jardin push` / `jardin pull`

Force one direction. `push` overwrites the server with local state; `pull` overwrites local
state with the server's. Use them when you know which side is right.

```sh
jardin sync
jardin pull
```

### `jardin spaces list` / `jardin spaces use`

Spaces are separate trees on the server. A machine syncs exactly one of them, or the common
tree. Switching resets `.sync-base.json`, so the next reconcile does not read the tree
switch as a mass deletion.

```sh
jardin spaces list
jardin spaces use studio
jardin spaces use --none
```

| Flag | What it does |
|---|---|
| `--none` | Clear the space and go back to syncing the common tree |

## Content

### `jardin memory search <query>` / `jardin memory index`

Case-insensitive substring search across every `.md` file under `memory/`, printed as
`path:line` followed by the matching line. `index` prints `index.md`, the router page.

### `jardin rules list` / `jardin rules edit <name>`

Rules are ordered policy files under `~/.jardin/rules/`. `edit` opens one in `$EDITOR`,
creating it if it does not exist. Emission order is `rule_order` from `~/.jardin.yml` first,
then everything else.

### `jardin skills list` / `jardin skills add <name>`

Skills are reusable agent capabilities under `~/.jardin/skills/`. `add` scaffolds a new one
with frontmatter, and refuses rather than overwriting an existing skill.

```sh
jardin memory search "traefik"
jardin rules edit 00-core
jardin skills add changelog
```

## Generating agent config

### `jardin install [agent] | --all`

Runs the adapters: `(rules + skills + machine) -> agent config`. With no agent and no
`--all`, it prints help rather than guessing.

```sh
jardin install claude
jardin install --all
jardin diff codex
```

| Agent | Writes |
|---|---|
| `claude` | `~/.claude/CLAUDE.md`, skills as `~/.claude/skills/<name>/SKILL.md`, plus a `SessionStart` hook and a `statusLine` in `~/.claude/settings.json` |
| `codex` | `~/.codex/AGENTS.md`, skills as `~/.codex/skills/<name>/SKILL.md` |
| `gemini` | `~/.gemini/GEMINI.md` |
| `hermes` | `~/SOUL.md` |
| `copilot` | `.github/copilot-instructions.md` |
| `cursor` | `.cursor/rules/<name>.mdc` |

The `~/.claude/settings.json` merge is additive: unknown keys and existing hooks survive, a
hook already mentioning `jardin recap` is left alone, and a `statusLine` you configured
yourself is never replaced. A re-install with nothing left to add writes nothing.

### `jardin diff <agent>`

Shows what `install` would change for that agent, without writing anything.

## Sessions

### `jardin sessions`

Lists the 20 most recent sealed session blocks: end time, project, duration, machine and
agent, branch, and output tokens.

### `jardin sessions live`

Shows what is running right now across every machine. Liveness is computed at read time — a
block is live when its last event is inside the 15-minute gap timeout and the machine's
heartbeat is recent.

### `jardin sessions scan`

Reads new agent activity out of the transcripts and folds it into blocks. The daemon runs it
before every sync.

| Flag | What it does |
|---|---|
| `--all` | Rebuild this machine's history from the full transcripts, ignoring stored offsets |

### `jardin stats`

Aggregates sealed blocks into a table.

```sh
jardin sessions scan --all
jardin stats --since 30d --by project
```

| Flag | Default | What it does |
|---|---|---|
| `--since` | `7d` | Window: `7d`, `30d`, `12h`, or `all` |
| `--by` | `project` | Group by `project`, `machine`, `agent`, `branch`, or `model` |

## Subscription limits

### `jardin usage`

Reports how much of each Claude subscription window is spent, per machine, with a bar and the
time until each window resets. The numbers come from Claude Code itself, so no credential is
needed — but nothing is recorded until `jardin install claude` has put the status line in
place and a session has made its first API call.

```sh
jardin usage
jardin usage --json
```

| Flag | What it does |
|---|---|
| `--statusline` | Read Claude Code's status-line payload on stdin, record it, print one line |
| `--live` | Cross-check against Anthropic's OAuth usage endpoint, if a token is available |
| `--json` | Emit this machine's snapshot as JSON, or every machine's when none matches |

`--statusline` is what Claude Code invokes; it renders on nearly every keystroke, so it never
fails the process and still prints a line when the payload is unusable. `--live` needs a token
(below), caches responses for 5 minutes because the endpoint rate-limits hard, and falls back
to the status-line snapshot when the token is rejected.

Freshness is derived on read, never stored: a window past its `resets_at` is shown as
last-observed rather than current, and a snapshot older than 15 minutes is marked stale.

With `usage_alerts` on in the dashboard's Settings, the server publishes a `usage_alert.created`
event to the Antenne the first time a window crosses `usage_threshold` (default 80), once per
window instance per account, not per machine — see
[configuration.md](configuration.md#antenne-settings). Jardin only publishes the event; Antenne
owns delivery.

### `jardin usage login` / `jardin usage logout`

Stores an optional OAuth token for `--live`. It is read from stdin only — never a flag, which
would land in the shell history and in `ps`.

```sh
claude setup-token | jardin usage login
jardin usage logout
```

It must be a subscription token from `claude setup-token`; a standard `sk-ant-api…` API key
cannot read subscription limits at all and is refused. The token goes to the OS keychain when
one is available, and only falls back to `usage_token` in `~/.jardin.yml` when it is not — see
[configuration.md](configuration.md). `logout` clears both, and warns when the environment
still sets one.

## Daemon

### `jardin daemon install` / `uninstall` / `status`

Installs a launchd or systemd service that ticks every 60 seconds: `sessions scan`, then —
only on machines where a usage token resolves — `usage --live`, then `sync`, then — at most every 5 minutes, gated by a `.last-install` stamp — `install` for
each configured agent. The launcher path is resolved through a stable symlink on `PATH`
rather than the versioned binary, so a Homebrew upgrade does not silently orphan the
service.

## Server

### `jardin serve`

Runs the sync API and serves the dashboard. See
[configuration.md](configuration.md) for the environment it reads and
[deployment.md](deployment.md) for how it is deployed.

```sh
jardin daemon install
jardin serve --port 9000 --data /srv/jardin
```

| Flag | Default | What it does |
|---|---|---|
| `--port` | `$PORT`, else `8420` | Listen port. Only overrides the environment when passed |
| `--data` | `$DATA_DIR`, else `~/.jardin` | Data directory |

## Maintenance

### `jardin update`

Self-updates to the latest release. Aliased as `jardin upgrade`. `--check` reports whether
an update is available without installing it.

```sh
jardin update --check
```

Back to the [documentation index](README.md).
