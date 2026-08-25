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

### `mycelium login [url]`

Authenticates this machine against a Mycelium server and writes `url`, `token` and `machine`
into `~/.mycelium.yml`. By default it signs in through the browser against the server's
identity provider: it opens a loopback port, sends you to `/api/auth/oidc`, and trades the
one-time code that comes back for a token — so a session already open with another Facile
tool completes the login without a second prompt. A server with no identity provider, or a
machine with no browser, falls back to device authorization: a user code, `/authorize`, and
polling until an admin approves. On success it also installs the background sync service.

The URL may be omitted once `MYCELIUM_URL` or a previous login has set one.

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
| `--no-browser` | Print the authorization URL instead of opening a browser, and use device authorization |
| `--no-daemon` | Do not install the background sync service |
| `--space` | Select a space to sync right after login, by name or id |

### `mycelium logout`

Revokes the session on the server and clears `token` from `~/.mycelium.yml`. Everything else in
that file — the server URL, the machine, the space, the rule order, the agent list and the
Anthropic usage token — is left alone, and running it while logged out is not an error. This
is the server session; `mycelium usage logout` forgets the unrelated Anthropic token.

### `mycelium status`

Prints the machine name, the sync URL, the selected space (`common` when unset), the rule
and skill counts, and this week's session total.

## Syncing

### `mycelium sync`

The normal path: a three-way reconcile against `~/.mycelium/.sync-base.json`. Local edits
push, remote edits pull, deletions propagate both ways, and an edit-versus-edit conflict
keeps the losing version at `~/.mycelium/.conflicts/<path>` rather than dropping it. The
copy sits outside the tree you read, keeps its real extension, and never syncs.

### `mycelium push` / `mycelium pull`

Force one direction. `push` overwrites the server with local state; `pull` overwrites local
state with the server's. Use them when you know which side is right.

```sh
mycelium sync
mycelium pull   # terminal only: it overwrites local files with the server's
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

Ranked search over `### finding` blocks under `memory/`, not whole pages and not substrings.
Results print as `path:line`, then the date the claim carries, then the block's heading and
first line. A block with no date prints none.

The server's hybrid search answers when it can and the local index answers otherwise, so a
search always returns something. `--verbose` says which half answered, `--local` skips the
server, `--limit` caps the list.

A `~~struck-through~~` claim is not indexed. The page keeps every word, but a sentence the page
goes on to retract will not answer a query. Ranking also decays with age, weakly: it breaks ties
between blocks that already match about equally and never overturns a better match.

`index` prints `index.md`, the router page.

### `mycelium memory log [path]` / `diff <ref> [path]` / `revert <ref> [path]`

What happened to memory, for a human asking. `log` lists changes newest first, each starting
with the ref that `diff` and `revert` take. A path narrows any of them, spelled the way search
reports it:

```sh
mycelium memory log conventions/no-slop.md
mycelium memory diff a1b2c3d4 conventions/no-slop.md
mycelium memory revert a1b2c3d4 conventions/no-slop.md
```

`revert` rewrites every file that existed at that ref and removes anything under the path that
did not, so it restores a state rather than only adding back. With no path the whole authored
tree moves, which is the answer to a sync that deleted more than it should have. What it
replaces is recorded first, so a revert to the wrong ref is itself revertible.

Nothing reaches the other machines until the next `mycelium sync`.

### `mycelium memory ratify [page...]` / `mycelium memory forget <page...>`

Most of the wiki is descriptive: a dated observation an agent filed, corrected freely when
better evidence turns up. A page carrying `type: standard` in its frontmatter is the opposite.
It says what a repository must do, so a change to it should land deliberately rather than
appear on every machine five minutes later.

`ratify` pins such a page at the content you just read, together with the machine and the day
you accepted it. With no arguments it prints where every normative page stands here:

```sh
mycelium memory ratify                      # standards/cli.md  CHANGED  accepted 2026-08-24 on lucy
mycelium memory ratify standards/cli.md     # accept the version now on disk
mycelium memory ratify --all                # accept everything unratified or changed
```

Four states, and only two of them are problems:

| state | meaning |
|---|---|
| `ratified` | the bytes on disk are the bytes you accepted here |
| `not ratified` | nobody has accepted this page on this machine yet |
| `CHANGED` | you accepted it once and it has moved since |
| `MISSING` | a page you accepted is no longer in the tree |

`CHANGED` and `MISSING` fail `mycelium doctor` and are the whole point. `not ratified` passes:
every fresh machine and every genuinely new standard starts there, and a check that failed on
a fresh install would be a line people learn to skip.

While a page is `CHANGED`, every `mycelium memory search` result from it is marked
`[changed since ratified]`, so the state reaches the reader at the point of use rather than
only in a report nobody ran.

**Ratification never gates anything.** The page still syncs, still exists, still gets read and
still ranks. Only its authority is in question, never its availability.

**Pins never leave the machine that made them.** Accepting a change on lucy says nothing about
ruche, which is the design rather than a limitation: if a pin travelled, one machine accepting
a wrong edit would clear the flag everywhere, and the flag is the thing that catches the edit.
It is the same choice `mycelium flow trust` makes, for a different reason — a flow gates
execution, a page gates authority.

The pin names content, not an event, so `mycelium memory revert` back to a version you had
already accepted makes the page `ratified` again with no second act. Reverting to any other
version leaves it `CHANGED`, correctly: nobody accepted that one here.

`forget <page>` drops the pin for a page that is genuinely gone. A pin outlives the page it
names — the store is authoritative, not derived — so a deleted standard reports `MISSING`
until someone confirms the deletion was meant.

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

## Flows

A flow is a recorded procedure: an ordered list of shell steps you replay instead of
re-deriving. Flows sync between machines; their run artifacts never leave the machine that
produced them. Full spec in [flow-v0.md](flow-v0.md) and [flow-v2.md](flow-v2.md).

### `mycelium flow list`

Every flow this machine has, with its step count and trust state.

```console
$ mycelium flow list
  ci-status      2 steps  trusted     Recent CI runs for the repo in the current directory.
  deploy-check   3 steps  not pinned  Build, then confirm the health endpoint answers.
  suite-check    2 steps  CHANGED     The repo's own quality gate, then filet.
```

`not pinned` means this machine has never approved the flow; `CHANGED` means it was edited
since it was approved here. Add `--json` for the machine-readable form.

### `mycelium flow add <name>`

Scaffolds `~/.mycelium/flows/<name>.yml` and nothing else — a scaffolded flow is not trusted,
so it still has to be reviewed and pinned before it can run.

### `mycelium flow trust <name>` / `mycelium flow untrust [name]`

`trust` prints the flow and asks for confirmation, then pins its checksum on this machine.
`--yes` skips the prompt, and is refused outside a terminal unless you pass it explicitly.

`untrust <name>` removes one pin. With **no argument** it prunes every pin whose flow file
has disappeared — the trust store is authoritative, not derived, so a deleted flow would
otherwise leave its checksum behind forever.

### `mycelium flow run <name>`

Runs the steps in order, streaming each one's output prefixed with its step name, and writes
a run artifact. A flow that is not trusted on this machine is refused before any step runs.

```console
$ mycelium flow run release
▸ Running release (2 steps)
[version] v0.12.0
[notify] shipping v0.12.0
✓ release: 2 steps in 412ms
  /home/yann/.mycelium/runs/release/2026-08-19T13-35-07.357725744Z.json
```

A step can read an earlier step's output. `needs` binds an environment variable to
`<step>.<field>`, where the field is `stdout`, `stderr` or `exit_code`:

```yaml
steps:
  - name: version
    run: git describe --tags --always
  - name: notify
    needs:
      VERSION: version.stdout
    run: curl -fsS -d "shipping $VERSION" https://hooks.example/notify
```

Values travel in the child's environment and are never spliced into the command, so a value
containing `; rm -rf /` arrives as those characters and nothing else. A chained value is capped
at 64 KB — write anything larger to a file and pass the path.

Steps that declare no `depends_on` run one at a time in file order, exactly as they always have.
Declaring it — even as an empty list — opts into the graph, and steps with no edge between them
run together:

```yaml
steps:
  - name: lint
    depends_on: []
    run: mise run lint
  - name: test
    depends_on: []
    run: mise run test
  - name: deploy
    depends_on: [lint, test]
    run: ./deploy.sh
```

A failed step blocks the steps that depend on it; independent branches finish. A step can also
declare `ephemeral: true` to keep its output out of the artifact, or `type:` to run a model
extension instead of a shell command. Full details in
[flow-composition.md](flow-composition.md); these fields need mycelium v0.14.0+ on every machine
that runs the flow.

### `mycelium flow query`

Searches every flow's history at once — the cross-flow question that `runs`
cannot answer:

```console
$ mycelium flow query --status failed --since 7d
  deploy-check  failed     2026-08-19T09:14:02Z      1.2s  at smoke
```

`--status`, `--since` (`7d`, `24h`, `all`), `--flow`, `--limit`, `--json`. It reads history, so a
flow you deleted still answers for what it did while it existed.

### `mycelium flow trust-model <type>`

Pins a model extension so typed steps may run it here. A model is TypeScript run by `bun` under
`~/.mycelium/extensions/models/`, and it syncs — so, like a flow, it prints itself for review and
runs nowhere until a person approves it on that machine. See
[flow-composition.md](flow-composition.md).

### `mycelium flow runs <name>` / `mycelium flow show <name> [id]`

`runs` lists a flow's recent runs — id, status, when, how long. `--limit` defaults to 20;
the last 50 are kept on disk per flow.

`show` prints one run in full: per-step exit codes, durations, output, and the values each
step received from earlier steps. With no id it shows the most recent run. Read this rather
than re-running to find out what happened.

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
| `agents` | `~/.agents/AGENTS.md`, skills as `~/.agents/skills/<name>/SKILL.md` — the cross-vendor standard path, read by nacelle and any tool following the AGENTS.md spec. Its `AGENTS.md` opens with a generated-by notice; `mycelium diff agents` first if the file may be handwritten |
| `claude` | `~/.claude/CLAUDE.md`, skills as `~/.claude/skills/<name>/SKILL.md`, plus a `SessionStart` hook and a `statusLine` in `~/.claude/settings.json` |
| `codex` | `~/.codex/AGENTS.md`, skills as `~/.codex/skills/<name>/SKILL.md` |
| `opencode` | `~/.config/opencode/AGENTS.md`, skills as `~/.config/opencode/skills/<name>/SKILL.md` |
| `gemini` | `~/.gemini/GEMINI.md`, skills as `~/.gemini/skills/<name>/SKILL.md` |
| `hermes` | `~/SOUL.md` |
| `copilot` | `.github/copilot-instructions.md` |
| `cursor` | `.cursor/rules/<name>.mdc` |

The `~/.claude/settings.json` merge is additive: unknown keys and existing hooks survive, a
hook already mentioning `mycelium recap` is left alone, and a `statusLine` you configured
yourself is never replaced. A re-install with nothing left to add writes nothing.

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

## Subscription limits

### `mycelium usage`

Reports how much of each Claude subscription window is spent, per machine, with a bar and the
time until each window resets. The numbers come from Claude Code itself, so no credential is
needed — but nothing is recorded until `mycelium install claude` has put the status line in
place and a session has made its first API call.

```sh
mycelium usage
mycelium usage --json
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
[configuration.md](configuration.md#antenne-settings). Mycelium only publishes the event; Antenne
owns delivery.

### `mycelium usage login` / `mycelium usage logout`

Stores an optional OAuth token for `--live`. It is read from stdin only — never a flag, which
would land in the shell history and in `ps`.

```sh
claude setup-token | mycelium usage login
mycelium usage logout
```

It must be a subscription token from `claude setup-token`; a standard `sk-ant-api…` API key
cannot read subscription limits at all and is refused. The token goes to the OS keychain when
one is available, and only falls back to `usage_token` in `~/.mycelium.yml` when it is not — see
[configuration.md](configuration.md). `logout` clears both, and warns when the environment
still sets one.

## Daemon

### `mycelium daemon install` / `uninstall` / `status`

Installs a launchd or systemd service that ticks every 60 seconds: `sessions scan`, then —
only on machines where a usage token resolves — `usage --live`, then `sync`, then — at most every 5 minutes, gated by a `.last-install` stamp — `install` for
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
