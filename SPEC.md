# Spec — memory versioning and memory quality

A cold-start handoff. Everything needed to execute with no prior conversation. The reasoning
behind each decision lives in `ROADMAP.md`; this file is the order and the exact files.

Written 2026-08-22.

## Goal

Give `~/.mycelium` a history so a page can be diffed, blamed and reverted, and make memory age
honestly so a stale claim stops outranking the fact that replaced it — without changing anything
an agent has to know.

## Why (evidence)

- **No undo.** `internal/sync/` reconciles by checksum, current state only. On 2026-08-19,
  `log.md:3` records `wiki reset: 246 pages deleted at the user's request`. There was no way to
  recover them, and there still is not.
- **No freshness signal.** Every finding carries `**Date**:` in prose. `internal/memory/rank.go`
  scores lexically with no recency term, so a superseded claim ranks exactly as well as its
  correction. `projects/nacelle.md:391` holds a struck-through claim that is still fully indexed.
- **Provenance is the only defence that survives.** Measured: the best content-based detector
  reaches 42.50% TPR on weak-signal memory poisoning, because a poisoned memory is semantically
  indistinguishable from a real one. Attacks succeed at 80–95% with a poison rate under 0.1%.

## Approach

Two independent tracks. **Track A** adds a `go-git` journal over the authored tree, committed
automatically inside existing write and sync paths, surfaced only through new read-only commands.
**Track B** promotes per-finding metadata from prose into a parseable block and teaches the
ranker to use it.

Both reuse what is already here. `internal/memory/chunk.go` already parses page frontmatter
(`frontmatter()`, `scalar()`) and already builds an enriched header per chunk (`chunkHeader()`) —
the metadata block is the same parsing pattern one level down, and the enrichment is what must be
preserved. `internal/sync/tree.go`'s `syncSkip()` already excludes dotfiles on both client and
server, which is what keeps `.git` out of the sync path with no new mechanism.

## Conventions checked

**Applies**: `filet` (`failOn: error`, `fileLines: 300`, `funcLines: 60`, `banInit`,
`banGlobalMutable`, `banInlineComments`, `requireDocComments`) — plan the code to pass, never
raise a threshold. `conventions/mycelium-agent-surface.md` — the agent-facing surface must not gain
a word. `conventions/multi-agent-git-hygiene.md` — commit explicit paths, this checkout is shared.

**Does not apply, stated so a worker does not look for it**: migrations (mycelium has no database),
auth/porte (nothing here touches identity), muse (no UI), events (nothing emitted),
module path (mycelium is flat `github.com/FacileStudio/Mycelium`, not the `apps/api` shape),
distribute (`go-git` is an ordinary Go dependency).

---

## Steps (ordered)

Steps 1–3 are independent of each other and of everything below. Start anywhere in them.

**Steps 1 to 10 have landed and shipped in v0.22.0**, released and deployed 2026-08-23. 1, 2 and
3 were in v0.20.0; 8 and 9 in v0.21.0; 4, 5, 6, 7 and 10 in v0.22.0. **Steps 11 and 12 are
open**, and 12 is still the largest thing on this page.

Track A is done: a page can be deleted and restored, and the history names the machine. Track B
has one code item left before consolidation, step 11.

**Read this before building on step 1.** Nothing writes the metadata block it parses. 0 of 476
chunks in the live wiki carry an `id`, a `supersedes` or a `confirmed`, because no rule asks an
agent to produce one. Step 10 works anyway, on the prose `**Date**:` line, and that is not a
stopgap: it covers 329 of 476 chunks and it is what the writing convention actually mandates.
See `ROADMAP.md` under B1 for why the block chose the wrong unit for supersession.

### 1. Per-finding metadata block  `[filet]`

**Done 2026-08-23.** Tests landed in `chunk_meta_test.go` rather than `chunk_test.go`, which was
close enough to the 300-line cap that adding them would have left no room.

`internal/memory/chunk.go`, `internal/memory/chunk_test.go`

Parse an optional HTML-comment block immediately under a `### ` heading into structured fields on
`Chunk`:

```markdown
### The line is domain versus storage
<!-- id: agent-surface-domain-vs-storage
     date: 2026-08-22
     source: decided with the user
     confirmed: 2026-08-22
     supersedes: agent-surface-nothing-below-crud -->
```

Reuse `frontmatter()` and `scalar()`'s shape rather than adding a YAML dependency. An HTML comment
because it is invisible in rendered markdown and cannot collide with page frontmatter, which must
stay the first line of a file.

Every field optional. A finding with no block parses exactly as today — this is additive, and the
existing corpus must keep working untouched.

**Exit**: `Chunks()` returns populated fields for a page with blocks and unchanged behaviour for a
page without; `go test ./internal/memory/` green; `filet check .` exit 0.

### 2. Secrets clause in the storage gate

**Done 2026-08-23.** The gate has a fourth condition and all 8 agent configs carry it.

`~/.mycelium/rules/` (the shared rules, not this repo), and this repo's `AGENTS.md` if it restates
the gate

The recommended write gate has four conditions and mycelium's has three. Add: **secrets never enter
memory — credentials and tokens are rejected outright.** `flow.go` already carries the equivalent
rule for flow files; memory does not.

**Exit**: the gate text names secrets; `mycelium install --all` regenerates every agent's config
with it.

### 3. Bulk-delete guard  `[filet]`

**Done 2026-08-23, wider than written below.**

`internal/sync/sync.go`, `internal/sync/client.go`, `cmd/sync.go`

A reconcile whose plan would delete more than `maxSilentDeletes` (start at 10) local files stops
before writing, reports the count and the paths, and requires `--force`. Versioning makes the
2026-08-19 deletion recoverable; this makes it not happen.

**Applies to both directions, counted against one limit.** This step originally guarded local
deletions only. Review found that the unguarded direction is the one nothing can undo yet: a local
wipe pushed up empties the copy every other machine pulls from, and step 4 has not landed. So
`plannedDeletes` collects removals both ways and refuses when the two together pass
`MaxSilentDeletes`. `BulkDeleteError` keeps them in separate fields, because "gone from this
machine" and "gone from the server" read as different accidents to whoever hits the refusal.

Two fixes came out of the same review and shipped with it: `ui.ErrorHint` keeps the whole refusal
on stderr, and `doctor`'s `last sync` check now fails past 24 hours. It used to report the age and
pass at any value, which is a hole only once a sync can stop and stay stopped.

**Exit**: a test drives a reconcile that would remove 11 files and asserts nothing was deleted and
a non-zero exit; `--force` performs it.

---

### 4. The journal  `[filet]`

**Done 2026-08-23.** Three deviations from what is written below, all deliberate.

**`Commit` takes no `paths` argument.** It stages the four versioned roots by name. A caller
passing the paths it happens to know about can miss one, and a missed page is exactly the
failure this track exists to prevent: a file created and deleted between two commits leaves no
trace at all. Naming four directories is still explicit staging, not add-all, and it is belt and
braces with the ignore file rather than depending on it.

**Five roots are versioned, not four.** `extensions/` holds the typed model code flows call. It is
authored, it syncs, and it is trust-pinned, so a bad reconcile loses it exactly the way it loses a
page. It is absent from the list below only because the directory did not exist when this was
written. `machines/` and `sessions/` are ignored along with the rest of the telemetry, and `/.*`
catches the root dotfiles by shape rather than by name.

**A note on `node_modules/`.** `extensions/models/package.json` means one `bun install` there
would create it. The ignore file names it so it can never be committed. **It is not excluded from
`syncSkip`, so it would still sync**, which was already true before the journal and is worth
fixing separately.

**The lock is a file lock, not a mutex.** The risk note below reads as an in-process race. It is
not one: `internal/daemon/daemon.go` runs `mycelium sync` as a child process rather than calling
into the package, and `client.Sync` has one call site. Two processes collide, so nothing in one
address space can see it. `internal/journal/lock.go` takes a non-blocking `flock` with a bounded
retry, mirroring `internal/sessions/lock.go`.

`journal.Inspect` backs a `history` check in `doctor`, because the journal introduced a state
that persists until a human acts: every commit failure is a warning on a sync that still
succeeds, so recording can stop and nothing else notices. That is the same hole the `last sync`
check had in v0.20.0, and the rule it produced applies here. A machine with no journal yet is
green, not red: every install that predates this is in that state until its next sync.

Measured on the live wiki, 42 pages and 70 tracked files: 51 ms for the first snapshot, 464K of
history, 25 ms per commit whether or not there is anything to commit.

`internal/journal/` (new package), `go.mod`

`github.com/go-git/go-git/v5`. **Not** a shelled-out `git`: the server image is
`gcr.io/distroless/static-debian12` with no shell, and the client ships as one static binary with
no runtime dependencies. Requiring a git binary for core memory regresses both.

- `Init(dataDir)` — `git init` if absent, idempotent, writes `.gitignore` covering `events/`,
  `usage/`, `claims/`, `runs/`, `.sync-base.json`, `*.conflict`, `tokens.json`.
- `Commit(dataDir, message string, paths []string)` — stages **explicit paths**, never add-all.
  Author from `config.LoadMyceliumConfig().Machine`.
- Version `memory/`, `rules/`, `skills/`, `flows/` only. The excluded set is high-churn telemetry
  that would bury the history.

**Do not touch `syncSkip()`.** Its `strings.HasPrefix(rel, ".")` is what keeps `.git` out of the
sync path, on both `internal/sync/tree.go` and `internal/server/sync_api.go`. Syncing a `.git`
directory through a file syncer is a documented corruption mode.

**Exit**: `~/.mycelium/.git` exists after `mycelium init`; `git log` in that directory shows commits;
`mycelium sync` produces exactly one commit per run; `events/` never appears in `git status`.

### 5. Commit on the write and sync paths  `[filet]`

**Done 2026-08-23. A sync commits twice, not once.**

Once before the reconcile, for anything written since the last one, and once after, describing
what moved. Without the first there is a window with no history at all: a page an agent wrote an
hour ago and a pull then deletes was never in a commit, so nothing can give it back. The
pre-commit finds nothing on almost every run and writes nothing. The two are different
operations, so "one commit per operation" still holds.

Only the second warns on failure. Both fail for the same reason and would print the same line,
and a warning that always arrives twice is one people stop reading.

Messages do not name the peer machine as the example below does. `sync.Result` carries counts and
paths, not which machine pushed what, and adding that to the wire for a commit subject is not
worth it.

`cmd/sync.go`, `internal/sync/sync.go`

One commit per operation, never per file. A `mycelium sync` that pulls six changes is one commit.

Messages are **mechanical and derived** — `sync: pulled 4 from lucy, pushed 2`. They serve one
reader: a human scanning `mycelium memory log`. **`mycelium sync` gains no arguments.** An earlier
draft wanted messages rich enough to replace `log.md`; see step 7 for why that is wrong.

**The journal must never block the work.** A corrupt repository, a held lock or a full disk must
still let the sync succeed. Log the failure through `Config.Logger` and continue — the same shape
as nacelle's `tools.Mycelium()` returning no tools when mycelium is absent rather than erroring.

**Exit**: a sync against a deliberately corrupted `.git` still reconciles and still exits 0.

### 6. Read-only history commands  `[filet]`

**Done 2026-08-23**, in `cmd/memory_history.go` rather than `cmd/memory.go`, which is 140 lines
and has no room for three more commands under the 300-line cap.

One thing had to be stripped to hold the line about the word `git`: a patch carries a
`diff --git a/x b/x` header, and `index <sha>..<sha>` under it. `neutralPatch` drops both, leaving
an ordinary unified diff, and `TestDiffNamesNoStorage` fails if either comes back.

`mycelium memory revert` snapshots the current state before replacing it, so a revert to the wrong
ref is itself revertible. A recovery command that cannot be recovered from is how one lost page
becomes two.

`cmd/memory.go`

`mycelium memory log [path]`, `mycelium memory diff <ref> [path]`, `mycelium memory revert <ref>`.
Register on `memoryCmd` beside the existing `search` and `index`.

These are for a human asking what happened to a page. **Nothing instructs an agent to use them**,
and no agent-facing help text, flag or output contains the word `git`.

**Exit**: a page can be deleted, then restored with `mycelium memory revert`, and `log` names the
machine that changed it.

### 7. `log.md` stays, and commits are never indexed

**Held, 2026-08-23.** Nothing indexes a commit: `readChunkDocs` reads pages and nothing else.

No code. This step is a decision to **not** write code, recorded so it is not undone.

`log.md` is the curated layer over a noisy mechanical one. Most commits will be
`sync: pulled 4 from lucy`; indexing them reimports exactly the noise the curation exists to
exclude. A finding's `###` heading states what is *true*; a log line states what *changed* and why
— a machine can derive the first from a diff and cannot derive the second.

**Exit**: `mycelium memory search` returns findings and `log.md` lines, and never a commit message.

### 8. `.conflict` files leave `memory/`  `[filet]`

**Done 2026-08-23.** The copy keeps its real extension, so `.conflicts/memory/a.md` opens as
markdown. `ConflictBackups` reads both this layout and the older sibling files, which are still
on machines that wrote one before the move, and `doctor` reports through it.

`internal/sync/conflict.go`

`writeConflictCopy()` writes `<path>.conflict` beside the page. A conflict as an event is domain
and `doctor` already reports it; a file called `foo.md.conflict` in the tree is storage leaking
into an agent's view. Write under `~/.mycelium/.conflicts/<path>` instead. Already excluded from
sync, so this is local clutter only.

Once step 4 lands, most stop being written: today's reconcile compares checksums and has no
common-ancestor **text**, so "edited on both sides" can only park a copy. Git objects supply a
merge base, so two agents appending different findings to one page merge cleanly. **Never write
conflict markers into a page** — that is Obsidian Git's most common complaint.

**Exit**: a forced edit-vs-edit conflict leaves `memory/` clean and the backup under `.conflicts/`.

### 9. Prune dot-directories in the tree walk  `[filet]`

**Done 2026-08-23.** `skipWalkDir` asks `syncSkip` one level up, with a trailing slash so
`runs` matches the `runs/` rule. The root is exempt: its relative path is `.`, which the dotfile
rule matches, and pruning it returns an empty tree that reads as every file having been deleted.

`internal/sync/tree.go`

`LocalTree()` skips dotfiles but returns `nil` rather than `filepath.SkipDir`, so it descends into
`.git` and stats every object before discarding each. Every sync gets slower as history grows.

**Exit**: a benchmark or timing check shows the walk does not read `.git` contents.

---

### 10. Recency in ranking  `[filet]`

**Done 2026-08-23, and step 1 turned out not to be the dependency this said it was.**

Two measurements changed the design. Both were taken against the live wiki, 476 chunks.

**The metadata block is written by nothing.** 0 chunks carry an `id`, 0 carry a `supersedes`, and
315 findings carry the prose `**Date**:` line that the writing convention in the shared rules
mandates. Step 1 shipped a parser for a format the corpus does not use, so a ranker reading only
`Meta` would have measured an empty set and reported that recency does not help. `Chunk.Date()`
reads `confirmed`, then `date`, then the prose line, taking the later of two dates on the five
lines that carry two. That brings 329 of 476 chunks into range. The remaining 147 are page
preambles with no date, and they are left at weight 1 rather than treated as ancient: absent is
not old, and penalising them would be a ranking change wearing a freshness costume.

**Supersession here is a sentence, not a block.** The convention is a `~~struck-through~~` claim
followed by `[SUPERSEDED by: ...]` and the correction, both inside the same `###` finding. So
down-ranking a chunk that says "SUPERSEDED" demotes the correction along with the claim, which is
backwards. `Chunk.Text()` drops struck spans from what is indexed and embedded instead, the same
call `takeMeta` already makes for the metadata comment. 11 chunks are affected, the page keeps
every word, and unstriking the text brings it straight back. `chunkDisplay` draws its excerpt from
the same text, so a hit no longer leads with the sentence the page exists to correct.

Measured: the query "commit message replaces the log.md entry removing one obligation" returned
the retracted sentence in `conventions/mycelium-agent-surface.md` and now returns a live claim on
another page.

**The decay itself is deliberately weak.** Exponential, half-life 180 days, floored at 0.85, so
recency moves a score inside a 15% band and can only break ties between chunks that already match
about equally well. On this corpus the weights run 0.99743 to 1.00000, because the wiki was reset
on 2026-08-19 and is five days old: age says almost nothing yet and the decay correctly does
almost nothing. It is built for the corpus in a year, not this one. Exponential rather than the
Gaussian that suits a date range, because a claim does not stop being true on a cliff.

`supersededIDs` resolves the block's forward pointer for the first finding that writes one. That
half is ready and unused.

`internal/memory/freshness.go` (new), `internal/memory/chunk.go`, `internal/memory/hybrid.go`,
`internal/memory/rank.go`

**Depended on step 1, and did not need it.** Combine lexical score with recency and supersession: decay the score, not
the data, so the decay is reversible and nothing is deleted. A chunk whose `supersedes` names it
as replaced, or whose `confirmed` date is old, ranks below its correction.

Zep reports up to 18.5% accuracy improvement on temporal-reasoning tasks from explicit validity
periods — the direction is measured, the number does not transfer.

**Exit**: `go test ./internal/memory/` including `eval_test.go` stays above `recallFloor = 0.60`,
and a test asserts a superseded chunk ranks below its replacement for a query matching both.

### 11. Wiki links reach the ranker  `[filet]`

`internal/memory/rank.go`, `internal/memory/chunk.go`

`[[page-name]]` links form a graph that retrieval currently ignores. Fuse an entity/link signal
into the score alongside BM25 and the vector half.

**Exit**: eval recall@5 does not regress; a test shows a page linked from a strong match gains.

### 12. Episodic-to-semantic consolidation  `[filet]`

`internal/daemon/daemon.go`, plus a new `internal/consolidate/`

The largest item and the last. Mycelium captures episodically (`events/`, sessions) and holds
semantic knowledge (`memory/`) with nothing between them — consolidation happens only when an
agent chooses to write a synthesis, which is one page in thirty today.

Asynchronous by design: capture raw cheaply, then a background stage reads recent episodes,
extracts candidate findings, consolidates against existing pages and **applies the storage gate**
before persisting. The daemon already ticks; this is a new stage inside it.

**Exit**: a daemon run over seeded events produces a candidate that passes the gate, and one that
fails it is dropped with a reason.

---

## Files to modify / new

| Path | Change |
|---|---|
| `internal/memory/chunk.go` | new — per-finding metadata block parsing (step 1); struck spans out of the indexed text (10) |
| `internal/memory/freshness.go` | **new** — the date a claim carries, and what age does to its score (10) |
| `internal/journal/` | **new package** — Init, Commit, the lock, log/diff/revert (4, 6) |
| `cmd/memory_history.go` | **new** — log, diff, revert (6) |
| `internal/config/config.go` | modify — MachineName, shared with cmd (4) |
| `internal/memory/rank.go` | modify — recency, supersession, link signal (10, 11) |
| `internal/memory/hybrid.go` | modify — pass the new signals through (10, 11) |
| `internal/journal/` | **new package** — Init, Commit (4) |
| `internal/sync/sync.go`, `client.go` | modify — bulk-delete guard, commit hook (3, 5) |
| `internal/sync/conflict.go` | modify — write under `.conflicts/` (8) |
| `internal/sync/tree.go` | modify — `SkipDir` on dot-directories (9) |
| `cmd/memory.go` | modify — `log`, `diff`, `revert` (6) |
| `cmd/sync.go` | modify — commit per sync, `--force` (3, 5) |
| `internal/ui/ui.go` | modify — `ErrorHint`, so a refusal stays on one stream (3) |
| `cmd/doctor.go` | modify — fail the `last sync` check once it is stale (3) |
| `internal/daemon/daemon.go` | modify — consolidation stage (12) |
| `internal/consolidate/` | **new package** (12) |
| `go.mod` | add `github.com/go-git/go-git/v5` |
| `~/.mycelium/rules/` | secrets clause (2) — outside this repo |

## Exit criteria

**All met on 2026-08-23** except the last line, which is a per-release check rather than a
one-time one.

- A page can be deleted and restored with `mycelium memory revert`, and the history names the
  machine that changed it.
- A mass deletion is refused without an explicit flag.
- `mycelium memory search` returns findings and `log.md` lines, never a commit message.
- **No agent-facing command, flag, help text or output contains the word `git`.**
- A search result reflects how old its claim is, and a superseded claim ranks below its
  replacement.
- `sh scripts/check.sh` green, `filet check .` exit 0, `filet test .` green.
- A sync against a corrupted `.git` still succeeds.

## Risks / unknown unknowns

- **go-git performance.** Pure-language git reimplementations have a poor reputation at scale;
  isomorphic-git is documented with "severe performance problems". At 30 pages and sub-megabyte
  this is irrelevant. Record the bound; do not be surprised at 10,000 pages.
- **Concurrency.** The daemon ticks roughly every 60s and a CLI command can run at the same
  moment. go-git does not lock for you. Serialise journal writes, or a commit races a commit.
- **The eval scores the live wiki.** `internal/memory/eval_test.go` reads `~/.mycelium/memory` and
  its 65 golden cases name real page paths. Any change to page layout breaks it for reasons
  unrelated to ranking. Use `fixture_eval_test.go` for experiments.
- **`filet` limits bite.** `fileLines: 300` and `funcLines: 60`. `internal/sync/sync.go` and
  `internal/memory/rank.go` are already substantial; budget for a file split rather than a
  line-shave, and never raise the threshold.
- **This checkout is shared with other agents.** Commit explicit paths. On 2026-08-22 an
  `add -A` swept another session's uncommitted work into an unrelated commit.

## Skip (YAGNI)

- **CRDTs.** They converge without conflict, which is wrong for a store of claims: a CRDT merges
  "X is true" and "X is false" and calls it done.
- **A vector database as the store.** Markdown stays canonical with search layered on top.
- **Content filtering for memory safety.** Measured at 42.50% TPR on the attacks that matter.
  Provenance and revert are the controls that survive; a filter is wasted work.
- **Whole-page retrieval.** Measured: 200-token chunks score 88.1% recall at 7.0% precision
  against 800-token at 87.9% and 1.4%. `SearchChunks` is already right.
- **One finding per file.** Rejected — see `ROADMAP.md`. The metadata is the whole motivation and
  step 1 delivers it in place, while splitting would discard the context enrichment `chunkHeader()`
  currently supplies for free, add naming and placement ceremony to every agent write, and cannot
  be A/B tested because all 65 golden cases name page paths.
- **Any agent-facing git verb.** No `memory commit`, no `--no-commit`, no "remember to commit".
