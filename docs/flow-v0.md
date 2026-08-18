# `jardin flow` — v0 spec

Flows are the executable half of memory. A skill tells an agent *why*; a flow
records *exactly what worked* so the next run costs nothing to re-derive.

v0 does one thing: run an ordered list of shell steps and keep a record of what
happened. No DAG, no templating, no typed models. Those are v2+ and the format
must not block them.

## Scope

In:

- `jardin flow run|list|show|runs|trust`
- YAML flow files under `~/.jardin/flows/`, synced like every other tree
- One JSON run artifact per execution under `~/.jardin/runs/`, never synced
- Trust-on-first-use so synced content cannot silently execute

Out (deliberately):

- Postgres. Sync is a three-way file merge and the server stores files on a
  volume; a database would give the CLI a dependency it cannot satisfy offline.
  Run artifacts are local JSON. If cross-machine history is ever wanted, the
  sync layer already knows how to distribute files.
- porte. Jardin's server auth is already scoped, hashed at rest, rate limited
  and constant-time. porte would buy SSO consistency with the suite, which is a
  real want and a separate PR — it does not make `flow` safer, because flow's
  threat is local execution of synced content, not authentication.

## File format

`~/.jardin/flows/<name>.yml`:

```yaml
name: deploy-check
description: Build, then confirm the health endpoint answers.
steps:
  - name: build
    run: mise run build
  - name: smoke
    run: curl -fsS https://x.facile.studio/health
    timeout: 30
    allow_failure: false
```

Rules:

- `name` must equal the filename stem. `flow list` refuses a mismatch.
- `run` is executed via `sh -c`, one step at a time, in order.
- `timeout` is seconds, default 300, hard cap 3600.
- `allow_failure` default false. A failing step ends the run unless set.
- No interpolation syntax exists in v0. See "Designing v2 out of a hole".

## Run artifact

`~/.jardin/runs/<flow>/<rfc3339-nano>.json`, mode 0600, directory 0700:

```json
{
  "flow": "deploy-check",
  "flow_checksum": "sha256:…",
  "machine": "ruche",
  "started_at": "2026-08-18T21:04:11.221Z",
  "finished_at": "2026-08-18T21:04:19.882Z",
  "status": "ok",
  "steps": [
    {
      "name": "build",
      "exit_code": 0,
      "duration_ms": 7412,
      "stdout": "…",
      "stderr": "",
      "truncated": false
    }
  ]
}
```

`status` is one of `ok`, `failed`, `timeout`. `flow_checksum` pins which
version of the flow produced the record, so history stays readable after a
flow is edited.

## Commands

| Command | Behaviour |
|---|---|
| `jardin flow list` | Flow names, step counts, trust state |
| `jardin flow run <name>` | Execute; stream step output; write the artifact; exit non-zero if the run failed |
| `jardin flow runs <name>` | Recent runs: timestamp, status, duration |
| `jardin flow show <name> [run]` | One run in full; defaults to the latest |
| `jardin flow trust <name>` | Print the flow, confirm, then pin. `--yes` for non-interactive |

`--json` on `list`, `runs` and `show`, because the primary caller is an agent.

## Security

The whole point of flows is that content arriving over the network becomes
code that runs on lucy and ruche. Today jardin distributes prose an agent reads
and can refuse; a flow gets handed to `sh`. These five items close that gap and
belong in v0, not a follow-up.

### 1. Trust on first use

`~/.jardin/.flow-trust.json` maps flow name to sha256 of its bytes. `flow run`
hashes the file first and refuses to execute when the hash is absent or
differs, naming the `flow trust` command to accept it.

The store keeps a hash, not the trusted content, so `flow trust` cannot show a
diff. It prints the whole flow file instead and asks for confirmation, and it
refuses outright when stdin is not a terminal unless `--yes` is passed. A
control whose entire value is human review must not be satisfiable by comparing
two hex strings nobody reads.

The store is a dotfile, and `syncSkip` in `internal/sync/sync.go` already
excludes every path starting with `.`, so trust is per machine for free. A
compromised server can push a flow; it cannot get it executed.

### 2. Runs never leave the machine

Add `strings.HasPrefix(rel, "runs/")` to `syncSkip`. Two reasons: artifacts are
unbounded where `.sync-base.json` is a 25KB manifest, and captured stdout is
exactly where a leaked secret would land.

### 3. Redact captured output

Before writing an artifact, replace the value of any environment variable whose
name matches `*TOKEN*`, `*SECRET*`, `*KEY*`, `*PASSWORD*` or `*CREDENTIAL*`
with `***`, case-insensitively, in both streams. Cheap, imperfect, and it
catches the common accident of a step echoing its own configuration.

### 4. The child cannot read the sync credential

Strip `JARDIN_TOKEN` from the child environment. A flow distributed by jardin
has no business reading the token that distributed it.

### 5. Execution hygiene

`exec.CommandContext` with the step timeout. 1MB cap per stream, then truncate
and set `truncated`. Run in the directory `flow run` was invoked from, recorded
in the artifact.

## `run:` is a launcher, not a program

Steps execute through `sh -c`, and `/bin/sh` is dash on ruche and bash 3.2 on
lucy. A bashism that works when typed interactively — `[[ ]]`, arrays, `local`,
`set -o pipefail` — breaks on one machine or the other.

So `run:` holds an invocation, never logic:

```yaml
steps:
  - name: sync-check
    run: bun ~/.jardin/skills/scripts/sync-check.ts
```

Anything with branching, JSON or error handling goes in a TypeScript file run
by bun. That also makes the step runnable outside jardin, which is what you
want when it breaks at 2am.

## Designing v2 out of a hole

v2 will want `${{ steps.build.stdout }}`. The unsafe version splices values into
the command string and hands the result to `sh`, which is command injection
with extra steps.

Keep the step struct at `Run string` plus `Env map[string]string` and nothing
else. When templating lands, resolved values are passed as environment
variables and referenced as `$VAR` inside `run`. The shell never sees an
interpolated string. Deciding this now costs nothing; deciding it later costs a
format migration.

## Implementation notes

New package `internal/flow`, split to stay under the 300-line file limit:

- `flow.go` — the `Flow` and `Step` types, parse and validate
- `store.go` — locate flows, read and write run artifacts
- `run.go` — the executor: timeouts, capture, truncation, redaction
- `trust.go` — the checksum pin

`internal/config` gains `FlowsDir()` and `RunsDir()`, matching the existing
one-line accessors.

`cmd/flow.go` holds the cobra tree, flat alongside its siblings.

**filet will fail the build until `filet.yml` lists the new package.**
`architecture.requiredFiles` is concrete files per package, not a glob, and
`arch.*` findings are `error` severity against `failOn: error`. Add:

```yaml
    "internal/flow": [flow.go, store.go, run.go, trust.go]
```

House style, all enforced: no inline comments, doc comments on exported
identifiers, no package-level mutable state, no `init()`, no TODO, functions at
or under 60 lines.

## Tests

`internal/flow` is pure enough to test without a shell where it matters:

- parse and validate: name mismatch, unknown field, timeout over the cap
- redaction: a secret in stdout is masked, a similarly named non-secret is not
- truncation: over-cap output is cut and flagged
- trust: unknown hash refuses, changed hash refuses, pinned hash runs
- executor, using `sh -c 'echo hi'` and `sh -c 'exit 3'`: exit codes, ordering,
  `allow_failure`, and a `sleep 5` against a 1s timeout

## Definition of done

`jardin flow run deploy-check` on ruche produces an artifact, `jardin flow runs
deploy-check --json` lists it, the flow file syncs to lucy, and the first run
there refuses until `jardin flow trust deploy-check` is accepted.
