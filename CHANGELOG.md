# Changelog

All notable changes to this project are documented here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While on
`0.x`, a breaking change bumps the minor.

Every entry below was reconstructed from git history on 2026-08-24, so it
records what shipped rather than what was written down at the time.

## [Unreleased]

## [0.25.0] — 2026-08-24

### Added

- The daemon consolidates episodes into memory. A new stage reads recent records from
  `events/<agent>/*.jsonl`, proposes candidate findings with deterministic heuristics,
  asks a local Ollama model whether each is still useful in 30 days (failing open to a
  heuristic-only verdict when it is unreachable), applies the storage gate, dedupes against
  the wiki via embeddings with a lexical fallback — superseding an existing claim only when
  similarity, judge agreement and a newer date all agree, failing closed to NOOP otherwise —
  and writes the survivors directly into `memory/` using the prose conventions: findings
  appended, old claims struck through rather than deleted. A watermark cursor makes runs
  idempotent; the stage runs at most once per hour and works fully offline.

- Nacelle transcripts count toward usage. `sessions scan` tails
  `~/.nacelle/sessions/*.jsonl` alongside the Claude ones, reading only `turn` records
  because `done` repeats the run total and would double every figure.

- Episodes can stay on one machine. Anything under `local/` is excluded from sync, so a
  harness can log raw conversation text to `local/events/<agent>/` and have consolidation
  read it without the text ever reaching the server.

### Fixed

- Consolidation no longer overwrites a wiki page. A new finding whose title kebabbed onto
  an existing filename replaced that page whole, including anything a human had written in
  it — the deduper only reports a match above its weak-similarity floor, so a page can
  exist that the decision knows nothing about. A collision appends now, like every other
  write in the stage. Superseding also strikes each paragraph separately, because one `~~`
  pair around a two-paragraph claim renders as half retracted and half still standing.

- A candidate carrying a credential no longer reaches the judge. The gate ran after the
  round trip, and `OLLAMA_URL` can name a host that is not this machine, so the secret rule
  fired only once the text had already left the process.

- The same event line always produces the same text. Object keys were walked in map order,
  so a record carrying two of `message`/`content`/`text` assembled differently on every run
  — and the cursor hash, the similarity score and the create-or-noop decision all read it.

- Half-written pages stay off the wire. A crash between writing `<page>.md.tmp` and
  renaming it left the fragment in `memory/` forever, and sync would have published it to
  every machine and indexed it as a whole page.

- A Nacelle record written without its trailing newline is counted. The offset stayed
  before it and the file was skipped for good once its size stopped changing; a torn write
  still waits for the rest, since it cannot parse as whole JSON.

- A failed consolidation run stamps its timestamp, so a broken Ollama or an unwritable
  memory directory no longer re-runs the whole pipeline on every 60-second daemon tick.
  `--force` still bypasses the wait.

## [0.24.0] — 2026-08-24

### Added

- Normative pages are ratified per machine. A page carrying `type: standard` is
  pinned in `~/.mycelium/.memory-ratified.json` at the checksum a human accepted,
  with the machine and the day. `mycelium memory ratify` accepts one, `forget`
  closes out a deletion, `mycelium doctor` fails on a changed or missing page, and
  `mycelium memory search` marks a result from a changed page. The pin never syncs
  by design: a travelling pin would let one machine accepting a wrong edit clear
  the flag everywhere, which is the propagation the check exists to catch.

### Fixed

- A page name quoted in backticks or inside a fenced block is no longer read as
  a wiki link, so a page documenting the syntax stops crediting everything it
  quotes. The eval helper had always dropped fences; the parser had not.

## [0.23.0] — 2026-08-24

### Added

- A wiki link now carries a query-conditioned credit into memory ranking, so a
  page linked from a strong hit is pulled up with it.
- `mycelium doctor` reports a golden set the retrieval eval cannot run, instead of
  the eval quietly grading nothing.
- The eval fixture corpus doubled, with hard cases and a link case set, and it
  grades both search paths.

### Fixed

- `mycelium flow` compares the work dir through `EvalSymlinks`, so the gate passes
  on macOS, where `/tmp` is a symlink.
- An eval-set name pointing outside the wiki counts as missing rather than
  passing.
- The eval's corpus floor is tied to the doctor models, so the two cannot drift
  apart.

### Removed

- The cross-language eval set. The corpus is English by decision, so grading
  French queries against it measured nothing.

## [0.22.0] — 2026-08-23

### Added

- Memory has a history it can be restored from: a journal of what changed, per
  finding.
- A search result shows how old its claim is, and ranking weighs how current a
  claim is rather than how well it reads.
- `mycelium doctor` reports a history that has stopped recording.

### Fixed

- Strikethrough inside backticks is no longer stripped, and `node_modules` never
  syncs.

### Changed

- The container image carries the version stamp, without `--dirty`.

## [0.21.0] — 2026-08-23

### Fixed

- Conflict copies are kept out of the pages an agent reads, so a reconcile no
  longer feeds both sides of a conflict into a context window.

### Changed

- Sync prunes excluded directories instead of walking into them.

## [0.20.0] — 2026-08-23

### Added

- A sync refuses a reconcile that would destroy more than ten files. Bulk
  deletion is now a decision, not an accident.
- Memory parses a per-finding metadata block under each heading.

### Fixed

- `mycelium doctor` fails the last-sync check once it goes stale, rather than
  passing on any timestamp at all.

### Changed

- Git hooks moved from tracked scripts to lefthook.

## [0.19.0] — 2026-08-22

### Changed

- Every agent that follows the AGENTS.md standard is served from one generated
  tree, instead of one adapter per agent.
- Gemini skills are written as files rather than inlined into `GEMINI.md`.

## [0.18.0] — 2026-08-20

### Changed

- Auth, the OIDC callback, the alert scan, the timeline, the usage read and the
  sync reconcile were untangled, each behind tests written first. No behaviour
  change was intended.

## [0.17.0] — 2026-08-20

### Added

- Retrieval has a gate that runs everywhere, graded against a committed corpus.
- Tests cover the flows and models routes, which shipped without any.

### Changed

- The oversized files were split: `internal/server` went from seven files to
  twenty-three, and the client and the rest of the Go tree followed.

## [0.16.1] — 2026-08-19

### Fixed

- A model's closure covers `require()` and backticked specifiers, not only
  static `import` strings.

## [0.16.0] — 2026-08-19

### Changed

- Pinning a model pins its imports, not just the file it names. A trusted model
  can no longer change by editing something it pulls in.

### Fixed

- The retrieval eval is skipped when its corpus is not on this machine, instead
  of failing the gate.

## [0.15.4] — 2026-08-19

### Fixed

- A model that takes no arguments receives an empty object rather than nothing.

## [0.15.3] — 2026-08-19

### Changed

- Rules and Skills are one Instructions entry in the dashboard rail.

## [0.15.2] — 2026-08-19

### Changed

- Flows and Models are one Automation entry in the dashboard rail.

## [0.15.1] — 2026-08-19

### Changed

- Flow and model handlers moved out of `server.go`.

## [0.15.0] — 2026-08-19

### Added

- A step can declare which of its values are secret, and they are redacted from
  the run artifact.
- `defineModel()` gives a model file the contract, so it stops being written by
  hand.
- The dashboard shows flows and models, read-only.

## [0.14.1] — 2026-08-19

### Fixed

- An ephemeral value stays ephemeral once something consumes it. It used to leak
  into the consumer's artifact.

## [0.14.0] — 2026-08-19

### Added

- The flow roadmap is finished: typed steps that run a model extension, a
  dependency graph via `depends_on`, and run history.
- The CLI-standard pass is complete, and the document it was measured against
  was corrected where the two disagreed.

## [0.13.2] — 2026-08-19

### Fixed

- A sync with nothing else to do prunes the stale backups, so they stop
  accumulating on an idle machine.

## [0.13.1] — 2026-08-19

### Fixed

- A machine no longer conflicts with itself over its own telemetry.

## [0.13.0] — 2026-08-19

### Added

- `needs` binds an environment variable to an earlier step's `stdout`, `stderr`
  or `exit_code`. Backward references only, and the value is data, never spliced
  into the command string.

## [0.12.0] — 2026-08-19

The largest release in the history: flows, memory ranking, semantic search and
the French-language check all landed here.

### Added

- `mycelium flow`: recorded procedures, pinned before they execute, with `flow
  add` to scaffold one, descriptions in `flow list`, and this machine's flows in
  the session recap. The trust gate is reviewable, and one bad flow no longer
  hides the rest.
- Memory search ranks with BM25 against a golden set, retrieves finding blocks
  rather than whole pages, and stops caring about word order. Ranking is
  reproducible and a pin can be dropped.
- Semantic search: embeddings via ollama, vectors stored flat or in qdrant, the
  server's lexical half ranked by chunk, and search from the dashboard. The
  index heals itself on a timer and indexes what is already there at worker
  start.
- A sync warns when it carries French, and never blocks on it. The check has one
  implementation, with a measured floor and a statement of what it cannot catch.
- Accent folding, so a French wiki answers an unaccented query.
- A claims coordination layer: task leases with scratchpads, injected in the
  recap, with a claims API, dashboard UI, an opencode adapter and a codex recap
  hook.

### Changed

- Install targets the agents this machine has, not every adapter.
- Landing and auth buttons harmonized on rounded corners and an SSO-first
  layout.

### Fixed

- `doctor` detects opencode, which it could not see before.
- Provenance is stripped from server excerpts too, not only local ones.

## [0.11.1] — 2026-08-11

### Added

- `mycelium doctor`, `mycelium skills validate`, and source frontmatter on skills.
  Feature work shipped under a patch bump.

## [0.11.0] — 2026-08-11

### Added

- The sessions dashboard gains tabs and cost tracking, plus a filet CI job.
- Monochrome brand logos in the landing adapters grid, and pi listed among the
  adapters.

### Fixed

- Grouping by model no longer shows `(none)` rows; cost model matching, stat
  card order and grid responsiveness corrected.

## [0.10.1] — 2026-08-11

### Added

- A canonical event store: the pi extension writes, Mycelium collects. Feature
  work shipped under a patch bump.
- The bus is configurable from the environment, and the variables are
  documented.

### Fixed

- A restarting server no longer produces "you are not an admin".
- `TRUSTED_PROXIES` is passed into the router instead of being inert, and
  Cloudflare is trusted as a proxy, so rate limits key on the visitor again.
  Picks up tronc v0.10.1, which stops trusting any `X-Forwarded-For`.

### Changed

- Favicons, icons, OG images and metadata harmonized across the suite.

## [0.10.0] — 2026-08-10

### Added

- The CLI signs in through the browser over a loopback SSO flow, with `logout`
  and environment overrides.

### Changed

- Installation delegates to the `facile` CLI, bootstrapped from
  `get.facile.studio`.

## [0.9.1] — 2026-08-08

### Security

- Admin scope is re-derived live instead of being trusted from a frozen session
  token, so revoking admin takes effect without waiting for the token to expire.

### Fixed

- Usage alerts key on the account, not the machine.

## [0.9.0] — 2026-08-08

### Added

- An alert fires on the Antenne when a usage window crosses its threshold.

## [0.8.0] — 2026-08-08

### Changed

- Breaking: Nook is renamed Antenne throughout.
- The server runs on tronc v0.8.0 and serves the client from one container.
- The client is rebuilt on the muse design system, with muse owning the anchors,
  the empty state and the sidebar collapse button.

### Added

- The dashboard charts subscription usage limits and tracks them, with the
  timeline and usage surface documented.
- The API reference is served at `/docs` through `tronc/apiref`.
- A quality gate runs before every push, and the curl installer gains
  `--no-color`.

### Fixed

- The pool emitter stops listening to clients it has already retired.
- The OIDC callback parses the error envelope and validates the token before
  redirecting.
- The HTTP server recovers from handler panics instead of dropping the process.

## [0.7.0] — 2026-08-04

### Added

- Live sessions: see what every machine is working on right now.

## [0.6.1] — 2026-08-04

### Security

- The common tree is owner-private rather than world-readable.

### Fixed

- Machine tokens are tied to users, so a machine can sync spaces.

## [0.6.0] — 2026-08-04

### Added

- Spaces, and Authentik SSO.

## [0.5.2] — 2026-08-04

### Added

- Canonical project identity, and emit hygiene on the event path.

## [0.5.1] — 2026-08-04

### Fixed

- Session scans take a `flock`, and shard reads are deduped by block id.

## [0.5.0] — 2026-08-04

### Added

- Session tracking: collect what an agent did, recap the context, publish it to
  Nook.

## [0.4.1] — 2026-08-03

### Fixed

- The daemon writes a stable binary path into its service unit, so an upgrade
  does not orphan the unit.

### Changed

- Leaf logo and favicon; the beehive metaphor is gone.

## [0.4.0] — 2026-08-03

### Changed

- Breaking: Ruche is renamed Mycelium.
- The README documents self-update and dashboard memory editing.

### Fixed

- Agent config distribution uses dir-based detection, logs a skipped sync, and
  writes to the Claude skills dir.

## [0.3.0] — 2026-06-23

### Added

- A memory editor in the dashboard, and `ruche update` for self-update.

## [0.2.0] — 2026-06-23

### Added

- `login` runs a browser device-authorization flow.

## [0.1.0] — 2026-06-23

First release, built and renamed in a single day: Hive became Ruche partway
through, and the on-disk vocabulary settled on `memory`.

### Added

- The CLI, a shared agent memory that syncs over HTTP, with a three-way
  reconcile so a concurrent edit does not silently lose data.
- A SvelteKit dashboard: machines with live connectivity, a memory file-tree
  browser and viewer, rules and skills as cards with detail pages, a landing
  page, and a copy-paste agent integration prompt.
- A background sync service, auto-enabled on login, that detects agents itself
  and tracks a token's last-seen time.
- A codex adapter that emits `~/.codex/skills/<name>/SKILL.md` instead of
  inlining the skill.
- YAML config at `~/.ruche.yml`, persisted tokens, and `ruche login`.
- GoReleaser v2 releases with a Homebrew tap.

### Security

- Tokens are hashed at rest and scoped, login is rate-limited, and the browser
  session token is hidden from the API tokens list and from the machines page.
  Session tokens are named per machine and rotate on re-login.
- Adapter output refuses to write through a symlink.

### Changed

- Cells are gone; there is one flat data dir. `sync_url` and `sync_token` are
  `url` and `token`, and the `RUCHE_` env prefix is dropped.

### Fixed

- `rule_order` is a hint, not an allowlist.
- A 401 clears the stored token, which breaks a dashboard redirect loop.
- Traefik gives API routes priority over the client, and the Docker build uses
  `golang:alpine`.

[Unreleased]: https://github.com/FacileStudio/Mycelium/compare/v0.23.0...HEAD
[0.23.0]: https://github.com/FacileStudio/Mycelium/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/FacileStudio/Mycelium/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/FacileStudio/Mycelium/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/FacileStudio/Mycelium/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/FacileStudio/Mycelium/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/FacileStudio/Mycelium/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/FacileStudio/Mycelium/compare/v0.16.1...v0.17.0
[0.16.1]: https://github.com/FacileStudio/Mycelium/compare/v0.16.0...v0.16.1
[0.16.0]: https://github.com/FacileStudio/Mycelium/compare/v0.15.4...v0.16.0
[0.15.4]: https://github.com/FacileStudio/Mycelium/compare/v0.15.3...v0.15.4
[0.15.3]: https://github.com/FacileStudio/Mycelium/compare/v0.15.2...v0.15.3
[0.15.2]: https://github.com/FacileStudio/Mycelium/compare/v0.15.1...v0.15.2
[0.15.1]: https://github.com/FacileStudio/Mycelium/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/FacileStudio/Mycelium/compare/v0.14.1...v0.15.0
[0.14.1]: https://github.com/FacileStudio/Mycelium/compare/v0.14.0...v0.14.1
[0.14.0]: https://github.com/FacileStudio/Mycelium/compare/v0.13.2...v0.14.0
[0.13.2]: https://github.com/FacileStudio/Mycelium/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/FacileStudio/Mycelium/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/FacileStudio/Mycelium/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/FacileStudio/Mycelium/compare/v0.11.1...v0.12.0
[0.11.1]: https://github.com/FacileStudio/Mycelium/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/FacileStudio/Mycelium/compare/v0.10.1...v0.11.0
[0.10.1]: https://github.com/FacileStudio/Mycelium/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/FacileStudio/Mycelium/compare/v0.9.1...v0.10.0
[0.9.1]: https://github.com/FacileStudio/Mycelium/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/FacileStudio/Mycelium/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/FacileStudio/Mycelium/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/FacileStudio/Mycelium/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/FacileStudio/Mycelium/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/FacileStudio/Mycelium/compare/v0.5.2...v0.6.0
[0.5.2]: https://github.com/FacileStudio/Mycelium/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/FacileStudio/Mycelium/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/FacileStudio/Mycelium/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/FacileStudio/Mycelium/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/FacileStudio/Mycelium/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/FacileStudio/Mycelium/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/FacileStudio/Mycelium/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/FacileStudio/Mycelium/releases/tag/v0.1.0
