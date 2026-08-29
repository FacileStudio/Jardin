# Changelog

All notable changes to this project are documented here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While on
`0.x`, a breaking change bumps the minor.

Every entry below was reconstructed from git history on 2026-08-24, so it
records what shipped rather than what was written down at the time.

## [Unreleased]

## [0.32.2] — 2026-08-29

### Fixed

- **`go test ./...` published a real artifact to the production server.** The MCP publish test
  pointed `MYCELIUM_URL` at `mycelium.facile.studio`, and `publish_artifact` syncs what it
  records. `DATA_DIR` was a temp dir but the credential was not: `LoadMyceliumConfig` reads
  `~/.mycelium.yml`, so the test authenticated as the developer's own machine and pushed a
  document titled "Inline Artifact" into the real store, which then synced everywhere. Three of
  them landed on 2026-08-29 before anyone noticed. The test now names a port nothing listens on,
  which exercises the same code and reaches nothing.

- **`artifact open` no longer puts prose on stdout.** The "no browser on this machine" note moved
  to stderr, so `mycelium artifact open <id> | pbcopy` copies the link and nothing else, as
  CLI-STANDARD §7.3 requires. A terminal still shows both lines.

- **`artifact add` syncs before it hands you the link.** It printed the server URL and opened a
  browser at it while the file was still on its way up, so a fast machine could race the upload
  and land on a 404. With no server configured it now prints the local path, which is the only
  thing that resolves there.

- **`cmd /c start <url>` reads a lone quoted argument as the window title.** Windows is not a
  release target today, but the opener now passes the empty title `start "" <url>` the way the
  documented invocation does.

## [0.32.1] — 2026-08-29

### Fixed

- **A machine with no browser gets the link, not an error.** `mycelium artifact open` shelled out
  to `xdg-open` whenever `DISPLAY` was set, so a container that inherits a display from its host
  without inheriting an opener died with a raw `exec: "xdg-open": executable file not found`, a
  usage dump, and a non-zero exit. The artifact was recorded and the link was never printed. Both
  `open` and `add` now print the link first and treat the browser as the optional part: a machine
  that has none says so on one dim line and exits `0`.

- **"Can this machine show a page" is one predicate, not two.** `internal/browser` answers it for
  the artifact commands and for `login`, which each carried their own version and each got a
  different half right. It now requires a display *and* an opener on `PATH`, so an SSH session
  into a Mac and a container without `xdg-open` both fall back the way a headless Linux box
  already did: `login` goes to the device flow rather than waiting three minutes for a browser
  that was never going to open.

## [0.32.0] — 2026-08-29

### Added

- **`mycelium artifact add` reads from stdin, and `open` goes to the hosted URL.** A page
  generated in a pipeline never has to touch a temp file, and an artifact recorded against a
  configured server opens at `https://<server>/artifacts/<id>` rather than at a local path
  nobody else can reach. `publish_artifact` returns the rendered content inline, so the model
  that recorded it can read back what it filed.

- **Journal logging in the backend and the dashboard.** The server ships its logs to Journal
  when `JOURNAL_URL` and `JOURNAL_TOKEN` are set, and the web UI connects through
  `@facile/journal` with credentials fetched from `GET /api/auth/config`, so a browser session
  never carries the service token.

- **An admin can promote another user to admin.** Bootstrapping a second administrator
  previously meant editing the database by hand.

- **Zoom and fullscreen for Mermaid diagrams in the artifact viewer.** A flowchart wider than
  the column was unreadable and had no way out of it. Fullscreen opens over a blurred backdrop
  and fits the diagram to the viewport on entry.

### Changed

- **A space is always explicit.** The unscoped common tree is gone from the client, and every
  request carries the space it belongs to. The switcher no longer offers a bucket whose
  contents depended on who was asking.

- **Artifacts wear `solar:file-smile-linear`** in the sidebar and on the empty state, instead
  of the generic card glyph shared with every other list.

- **muse pinned at v0.7.0, with `WordReveal` vendored into the client.** muse dropped the
  component as its only GSAP-plugin consumer and names Mycelium as the sole user, so it moves
  here with `gsap` as a direct dependency.

### Fixed

- **Space access is restricted to members, and the admin bypass is removed.** An admin could
  read every space on the instance regardless of membership. Membership is now the only thing
  the server checks.

- **The markdown palette maps onto muse tokens.** muse v0.7.0 sets `--color-*: initial`, which
  unsets Tailwind's stock palette, so 57 utilities in `MarkdownMuse.svelte` emitted no CSS at
  all: callouts, diff badges, status pills and highlights rendered as unstyled text. Nothing
  errored and the build stayed green, which is how it nearly shipped.

- **An admin can select their personal space again.** The switcher suppressed the row by
  passing `undefined`, which Svelte resolves to the prop's default, so the row rendered for a
  non-admin and `pickSpace` then refused the click. `null` is the suppression signal.

- **`publish_artifact` syncs the artifact to the server.** It wrote the file locally and left
  it there until the next sync tick, so the URL it returned 404ed for the first minute.

- **Alerts parse full markdown, and a Mermaid SVG is no longer capped.** Alert bodies rendered
  their markup as literal text, and the diagram was clamped to a width narrower than its own
  viewBox.

- **Null lists no longer break space switching in the CLI.** An empty response decoded to a nil
  slice and every caller assumed a list.

## [0.31.0] — 2026-08-27

### Added

- **Native Mermaid diagram rendering in web markdown viewer.** Integrated client-side Mermaid flowchart rendering with syntax error fallbacks and a source/diagram toggle in `MarkdownMuse.svelte`.
- **`publish_artifact` MCP tool.** Registered as the primary MCP tool for recording artifacts, retaining `publish_report` as a compatibility alias.

### Changed

- **Renamed reports to artifacts across the full stack.** Reorganized `internal/reports` into `internal/artifacts`, mounted `/api/artifacts` HTTP endpoints with `/api/reports` compatibility aliases, and updated web routes to `/artifacts`.

### Added

- **Web dashboard reports viewer and API endpoints.** Added `/api/reports` and `/api/reports/{id}` in the Go server, plus a full Reports view (`/reports` and `/reports/[id]`) in the SvelteKit dashboard with `@facile/muse` components, preview/source toggle, and deletion support.

## [0.29.0] — 2026-08-27

### Added

- **`people/` memory directory and person classification.** `mycelium init` scaffolds `memory/people/` alongside `standards/`, `classifyDir` routes person-related findings to `people/`, and the web dashboard displays `people` and `standards` folders.

## [0.28.0] — 2026-08-26

### Added

- **`mycelium report` records a rendered page and carries it to the machine you are sitting
  at.** An agent working over SSH on a headless box can produce an HTML report and has
  nowhere to put it. `mycelium report add <file>` writes it into `~/.mycelium/reports/`,
  which syncs like the rest of the tree, so it opens on the machine with a screen. It is
  stored and never hosted: a file opened from disk gets an opaque origin and cannot read the
  bearer token the web UI keeps in `localStorage`, which is the whole reason there is no URL
  to hand anybody. The identifier comes from the document's `<title>`, so recording the same
  page twice replaces it rather than piling up copies, and a report expires after thirty days
  unless `--expires never` pins it. Metadata rides in the file as meta tags rather than in a
  sidecar, because sync reconciles one file at a time and a shared manifest would conflict
  every time two machines recorded a report in the same minute. `add` names any relative
  `src` or `href` it finds, since a page opened from disk cannot fetch its siblings and a
  missing stylesheet is a failure only the reader ever sees.

- **A report never enters the wiki.** Every indexer, embedder and language scan is already
  scoped to `memory/**.md`, so `reports/` crosses machines without joining the corpus a
  search answers from. The wiki holds the fact as text and keeps it; a report is the picture
  of one and expires.

- **`publish_report`, a fourth MCP tool.** Its description carries the trigger as well as the
  behaviour, because that string is the only text guaranteed to be in front of a model at the
  moment it decides. It says what a report is not, twice: not a link, and not a finding.

### Changed

- **`/api/sync/files/{path}` answers `application/octet-stream` with `nosniff`.**
  `http.ServeFile` types a response from the file's extension, so the endpoint would have
  answered a synced `.html` as `text/html` on the origin whose `localStorage` holds the token
  that mints API tokens and writes `rules/`. Auth there is header-only, so a browser
  navigation gets a 401 and the path was never reachable that way. Presetting the type is
  what keeps that true now the tree carries agent-authored HTML, and it leaves ServeFile's
  Range and If-Modified-Since handling intact because ServeFile only sniffs when the header
  is absent.

## [0.27.0] — 2026-08-26

### Added

- **Agents reach memory and flows as tools, not as instructions.** `mycelium mcp` speaks
  MCP over stdio and serves three of them: `search_memory`, `list_flows` and `run_flow`.
  `run_flow` takes a flow name and nothing else, because a fixed set of procedures a human
  read and pinned is the whole defence against the injection this kind of server usually
  ships with, and an unpinned flow comes back with the command that pins it rather than an
  exit code. Stdio is not a default here but the only transport that can work: the steps run
  `sh -c` on the calling machine and the trust pin is per machine.

- **`mycelium install` declares that server in each assistant's own config**, merging into
  `~/.claude.json`, `~/.gemini/settings.json` and `~/.config/opencode/opencode.json` rather
  than overwriting them, and replacing the entry mycelium owns whatever name it carries so
  the pre-rename `jardin` entry stops pointing at a deleted binary. Codex is skipped on
  purpose: its MCP servers live in TOML. A rule now renders for one audience or the other,
  so an assistant is told to call `search_memory` or to run `mycelium memory search`, never
  both.

- **`mycelium memory add` files a finding and everything that goes with it in one call**: the
  finding on its page, that page's `updated:` stamp, the pointer in `index.md` and the line
  in `log.md`. All four or none. Done by hand the one that gets skipped is always the index,
  because it is the only edit that is not where the writing happened. `--body-stdin` takes
  prose that would otherwise be a fight with shell quoting, and the log line is an argument
  rather than something derived from the diff, because it records what was wrong before and
  a diff does not carry that.

- **`mycelium doctor` reports the MCP declaration.** An assistant whose config will not parse
  is refused rather than overwritten, its rules quietly render for the CLI, and the result
  looks exactly like an assistant that was never meant to have the tools. The check names
  the file and fails, and it separates that from an assistant install has simply not reached
  yet.

- **A failed background sync announces itself on Linux.** The generated systemd unit carries
  `OnFailure=`, where before a nonzero exit only marked the unit failed in systemd's own
  state and nothing read it until somebody typed `journalctl`. launchd already redirected to
  `~/.mycelium/daemon.log` and needs no equivalent.

- **One mark, generated rather than hand-edited.** `scripts/icons.ts` writes all six brand
  assets from a single glyph. They had drifted into three different marks, and the card that
  renders wherever the site is shared still advertised a hostname that answers 404.

### Changed

- **`mycelium sync --force` and `mycelium pull` only run from a terminal.** Both hand a
  machine the power to empty its own wiki in one call, and on 2026-08-25 a reconcile deleted
  102 files here because the guard that stopped it printed the flag that waived it. An agent
  whose goal is a finished sync reads that sentence as the next step. The refusal a
  non-interactive caller gets now carries no flag at all, and a refused bulk delete states
  how many files each side holds, which is the line that separates a cleanup somebody meant
  from a server that lost its data volume.

- **`mycelium claim start` asks the server before taking a claim.** Local claim files arrive
  on a daemon tick, so two agents on two machines could both read "no claims" for up to a
  minute and both take the repo. An unreachable server downgrades the verdict instead of
  ending the command: the claim is taken and reported as unverified, because a lock that
  fails closed would stop the work every time a laptop was on a train.

- **Sync staleness is measured in daemon ticks, not in a round number of hours.** The daemon
  runs every 60 seconds and the old threshold was 24 hours, a gap of 1440x, so sync failed
  for twenty hours on 2026-08-25 while `doctor` printed a green tick beside
  `last sync: 19h43m36s ago`. It is thirty ticks now when the service is installed and still
  a day when syncing is something a person does by hand, and `mycelium recap` prints the
  same line at the start of a session, only when it is stale.

- **A browser session from a password login expires and evicts the one it replaces**, which
  is what every SSO session already did. Until now that path minted an admin credential that
  never expired, which is also why it slipped past the fix below.

### Fixed

- **Login sessions are no longer listed as API tokens.** The settings page compared a whole
  token name against `session`, which matches neither `session:<email>` nor `cli:<email>`, so
  every browser and CLI login anyone had ever made appeared as an API token with a revoke
  button beside it. The filter moved to the server, where every client gets it right at once,
  and it tests the expiry rather than the name.

- **A page being written can no longer reach the server half-landed.** Every wiki write is
  staged beside its target and renamed into place, but the staged file was named so that sync
  did not recognise it as temporary. A crash between the two would have published a whole
  second copy of the page to the server and to every other machine.

- **Two agents filing a finding at the same moment no longer overwrite each other.**
  `index.md` and `log.md` are read, appended to and written back, and an agent files a
  finding when its task ends, which is exactly when several finish at once. The read and the
  write now happen under one lock. Sync deliberately does not take it: serialising a
  reconcile against a write would put a network round trip between an agent and its own
  finding.

- **An index pointer is named after the page it points at.** It carried the page's slug where
  every hand-written line in the corpus carries a title, so a filed entry stopped being
  scannable among the others.

- **`mycelium install claude` cannot leave the account record truncated.** `~/.claude.json`
  holds project history and the account record, is not regenerable, and is rewritten by any
  running session. It is now replaced through a rename rather than opened with `O_TRUNC`,
  which closes the window where a crash lost all of it.

## [0.26.0] — 2026-08-24

### Changed

- **Breaking.** The name is now Mycelium, and every user-visible surface moved with it. The
  binary is `mycelium`, the data directory is `~/.mycelium/`, the credentials file is
  `~/.mycelium.yml`, the environment variables are `MYCELIUM_URL`, `MYCELIUM_TOKEN`,
  `MYCELIUM_SERVER_URL`, `MYCELIUM_USAGE_TOKEN` and `MYCELIUM_VECTOR_SEARCH`, the Go module
  path is `github.com/FacileStudio/Mycelium`, and the server answers on
  `mycelium.facile.studio`. There is no compatibility shim: a machine set up before this
  release has to reinstall and log in again, and a self-hosted deployment has to move its
  data volume and update its OIDC application slug and redirect URL.

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

[Unreleased]: https://github.com/FacileStudio/Mycelium/compare/v0.28.0...HEAD
[0.28.0]: https://github.com/FacileStudio/Mycelium/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/FacileStudio/Mycelium/compare/v0.26.0...v0.27.0
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
[0.26.0]: https://github.com/FacileStudio/Mycelium/compare/v0.25.0...v0.26.0
[0.5.2]: https://github.com/FacileStudio/Mycelium/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/FacileStudio/Mycelium/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/FacileStudio/Mycelium/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/FacileStudio/Mycelium/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/FacileStudio/Mycelium/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/FacileStudio/Mycelium/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/FacileStudio/Mycelium/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/FacileStudio/Mycelium/releases/tag/v0.1.0
