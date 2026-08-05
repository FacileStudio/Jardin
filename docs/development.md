# Mycelium — Development

Local setup, the test suite, and the quality gate that runs before every push.

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| Go | 1.26 | `go.mod` declares `go 1.26.1`; `mise.toml` pins the toolchain to `1.26` |
| bun | 1.x | Builds and type-checks the SvelteKit dashboard |
| mise | any | Task runner. Everything it runs is also runnable by hand |

## Setup

```sh
mise run hooks
mise run install
```

`mise run hooks` runs `git config core.hooksPath .githooks`, which is what enables the
pre-push gate in this clone. Git does not do it for you when you clone.

`mise run install` runs `bun install --frozen-lockfile` in `apps/client`.

## Build and run

```sh
go build -o mycelium .
./mycelium --version
```

The CLI needs nothing else — `mycelium init` scaffolds `~/.mycelium` and every command works
offline until you `mycelium login`.

To run the server against a throwaway data directory:

```sh
go run . serve --data /tmp/mycelium-dev --port 8420
```

With neither `PASSWORD` nor `OIDC_ISSUER` set and `APP_ENV` left at `development`, every
request is served as admin and the server logs a warning saying so. That is fine locally
and refused outright in production.

For the dashboard, run Vite separately:

```sh
cd apps/client && bun run dev
```

The Go binary only serves the SPA when `CLIENT_DIR` points at a directory containing an
`index.html`, so a `go run . serve` with no build present simply skips the catch-all and
serves the API alone.

## Tests

```sh
go test ./...
```

The suite is fast and hermetic — no database, no network. Worth reading before changing
anything in these areas, because the tests carry the intent:

| File | Covers |
|---|---|
| `internal/server/router_test.go` | Unknown `/api` paths return a 404 envelope, not the SPA |
| `internal/server/server_security_test.go` | Token scopes, traversal guards, rate limits |
| `internal/server/spaces_test.go` | Membership guard and the common-tree fence |
| `internal/server/device_test.go` | Device authorization lifecycle and single-use tokens |
| `internal/server/emitter_test.go` | Pending-block selection, ledger, envelope shape |
| `internal/sync/sync_test.go` | Three-way reconcile, deletions, conflict backups |
| `internal/sessions/sessions_test.go` | Gap-based sessionization, sealing, dedupe |
| `internal/adapter/*_test.go` | Adapter output and the Claude hook merge |

## The quality gate

`scripts/check.sh` is the gate. It reports and never rewrites, except with `--format`.

```sh
sh scripts/check.sh             # gofmt -l, go vet, go test, then the client type-check
sh scripts/check.sh --go-only   # Go only
sh scripts/check.sh --format    # rewrite Go sources in place
```

Through mise:

```sh
mise run check
mise run check-go
mise run format
```

Two details worth knowing before you change the script:

- **The pre-push hook calls the script directly**, not through mise. `mise run` resolves
  every tool in the merged config before running any task body, so an unrelated broken tool
  in your global mise config would take the gate down with it.
- **The script resolves the toolchain from `GOROOT`** when it is set. mise exports `GOROOT`
  for the pinned version but leaves an unrelated `go` earlier on `PATH` — Homebrew's,
  typically — and a `go` binary driving a different `GOROOT` fails with
  `compile: version "X" does not match go tool version "Y"`.

The client half is skipped with a message when `bun` is not on `PATH`, so the gate still
runs usefully in a Go-only environment.

## Bypassing the hook

```sh
git push --no-verify
```

Only for a failure that is genuinely unrelated to your change. If the gate is red on `main`,
fix the gate.

## Conventions

- No inline comments in code. Comments explain *why* something is the way it is, above the
  declaration, and only when the reason is not obvious from the code.
- Adapters are pure functions and self-register through `init()`. Adding an agent is one
  file in `internal/adapter/`.
- Read chi path parameters with `pathParam(r, key)`, never `chi.URLParam` directly. See
  [architecture.md](architecture.md) for why.
- The copy-paste master prompt shown in the dashboard lives in
  `apps/client/src/lib/agentPrompt.ts`.

Back to the [documentation index](README.md).
