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

**Steps 1, 2, 3, 8 and 9 landed on 2026-08-23**, in v0.20.0. Steps 4 to 7 and 10 to 12 are open.
Step 4, the journal, is the one to do next: the guard in step 3 stops a mass deletion, but
losing five pages is still permanent until there is a history to restore them from.

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

`cmd/memory.go`

`mycelium memory log [path]`, `mycelium memory diff <ref> [path]`, `mycelium memory revert <ref>`.
Register on `memoryCmd` beside the existing `search` and `index`.

These are for a human asking what happened to a page. **Nothing instructs an agent to use them**,
and no agent-facing help text, flag or output contains the word `git`.

**Exit**: a page can be deleted, then restored with `mycelium memory revert`, and `log` names the
machine that changed it.

### 7. `log.md` stays, and commits are never indexed

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

`internal/memory/rank.go`, `internal/memory/hybrid.go`

**Depends on step 1.** Combine lexical score with recency and supersession: decay the score, not
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
| `internal/memory/chunk.go` | new — per-finding metadata block parsing (step 1) |
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
