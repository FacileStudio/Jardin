# Roadmap

Written 2026-08-22. A cold-start handoff: enough to pick up a track with no prior
conversation. **The executable form is `SPEC.md` — exact files, exact steps, exit criteria per
step.** This file holds the why and the ordering; the reasoning behind each item lives in the commit that
closes it, and the research behind both tracks is in
`~/.mycelium/memory/syntheses/agent-memory-practice-2026.md`.

## Where this is

Memory, rules, skills, flows, sessions and claims all ship. The `agents` adapter landed in
v0.19.0, so `~/.agents/AGENTS.md` and `~/.agents/skills/` are generated for every tool that
follows the AGENTS.md specification.

Two things are missing, and they are the two tracks below. **Memory has no history**: the sync
is a three-way reconcile by checksum, current state only, so a page cannot be diffed, blamed or
rolled back. On 2026-08-19 that cost 246 pages with no way to recover them. And **memory has no
freshness signal**: every claim carries a write date that nothing surfaces at retrieval, so a
stale page outranks a fresh one on wording alone.

## Start here

The tracks below are in logical order, not priority order. If you have one evening, these
three are hours rather than weeks and each one stands alone:

1. **B1, `supersedes` in frontmatter.** One key. Today a superseded claim ranks exactly as
   well as the claim that replaced it, and nothing can follow the chain. Smallest change on
   this page, largest effect on what an agent actually reads.
2. **B5, the secrets clause.** One sentence in the storage gate. Flows already carry the rule.
3. **A3, the bulk-delete guard.** The difference between the 2026-08-19 deletion being
   recoverable and it not happening.

Then A1 and A2, which everything about recovery, blame and poisoning defence depends on.
B6 is the largest and belongs last.

## Track A — give memory a history

The agent-facing surface must not change. Agents keep writing files and running `mycelium sync`;
they never learn that git exists. See `~/.mycelium/memory/conventions/mycelium-agent-surface.md`.

### A1. A go-git journal over the authored tree

`internal/memory/`, or a new `internal/journal/`

`go-git`, not a shelled-out `git`: the server image is `gcr.io/distroless/static-debian12` with
no shell, and the client installs today as a single static binary with no runtime dependencies.
Requiring a git binary for core memory would regress both.

Version `memory/`, `rules/`, `skills/`, `flows/`. Exclude `events/`, `usage/`, `claims/` and
`runs/` — they are high-churn telemetry and would bury the history in noise.

`syncSkip` already excludes anything starting with `.` on both client and server, so `.git`
inside the data directory never syncs. That is the failure mode that corrupts git repos kept in
Dropbox, and it is already fenced. Do not remove that filter.

One commit per operation, never per file: one `mycelium sync` that pulls six changes is one
commit. Author from the machine name the token already carries. Stage explicit paths, never
add-all.

### A2. The journal must never block the work

A corrupt repository, a held lock or a full disk must still let the memory write and the sync
succeed. The failure goes to the human, never to the agent — the same shape as `tools.Mycelium()`
in nacelle returning no tools when mycelium is absent rather than erroring.

### A3. A bulk-delete guard

`internal/sync/`

A reconcile that would remove more than a handful of pages refuses and asks. Versioning makes
the 2026-08-19 deletion recoverable; this makes it not happen.

### A4. Commit messages are mechanical, and that is enough

**`mycelium sync` gains no arguments.** A commit message here serves one reader: a human scanning
`memory log` to find when something changed. `sync: pulled 4 from lucy, pushed 2` and
`memory: 3 pages changed under conventions/` are complete descriptions of what those commits
are, and both derive from the diff with no agent involvement.

An earlier draft wanted derived messages rich enough to replace `log.md`. They are not, and they
do not need to be — see A5. Deriving a mechanical message for a forensic log is easy; deriving a
narrative one is impossible, and only the first is required.

### A5. `log.md` stays, and commits are never indexed

`internal/memory/`

**`log.md` is part of the design, not a stand-in.** An earlier draft of this roadmap proposed
deriving commit messages from the diff and deleting it. That was wrong twice over.

First, derivation loses the information that makes it useful. A finding's `###` heading states
what is *true*; a log line states what *changed*, what was wrong before, and what still holds.
Compare, from the same edit:

```text
heading:   The line is domain versus storage, not surface versus internals
log line:  lint | mycelium-agent-surface: corrected — sync is domain, not a leak; agents keep
           `mycelium sync` and reads stay explicit. Only .conflict files in memory/ and
           hand-written log.md actually leak storage
```

Overlapping words, different information. A machine can produce the first from a diff and
cannot produce the second.

Second, this is the changelog argument and it was settled a decade ago: a commit log records
every mechanical step, and most of mycelium's will be `sync: pulled 4 from lucy`. `log.md` is
signal precisely because something was excluded. Keep a Changelog's framing transfers directly
— one records how the state moved, the other records why it mattered.

**Therefore commits are never indexed into search.** Indexing them reimports the noise the
curated log exists to exclude. Search reads pages and `log.md`; git is a forensic layer for a
human asking "who changed this and when", reached through `memory log` / `diff` / `revert` and
through nothing an agent runs.

`log.md` keeps its shape: one dense dated line per change, continuous rather than grouped by
release, written at the time. It is closer to a lab notebook than to a CHANGELOG, and it is
optimised for keyword retrieval rather than for reading top to bottom.

### A6. Move `.conflict` files out of `memory/`

`internal/sync/conflict.go`

A conflict as an event is domain and `doctor` already reports it. A file called
`foo.md.conflict` sitting in the tree is storage leaking into the agent's view. Write it under
`~/.mycelium/.conflicts/` instead. Already excluded from sync, so it is local clutter only.

Once A1 lands, most of these stop being written at all: today's reconcile compares checksums and
has no common-ancestor **text**, so "edited on both sides" can only park a copy. Git objects
supply a merge base, so two agents appending different findings to one page merge cleanly.

### A7. Prune dot-directories in the tree walk

`internal/sync/tree.go`

`LocalTree` skips dotfiles but does not return `filepath.SkipDir`, so it descends into `.git`
and stats every object before discarding each one. Every sync gets slower as history grows.

## Track B — make memory age honestly

**B1 and B2 are one change, not two.** Both want per-claim metadata that page-level frontmatter
cannot express, and both are delivered by the metadata block in `SPEC.md` step 1. B3 depends on
that block existing.

### B1. Per-finding metadata, in the page

An HTML-comment block under each `### heading` carrying `id`, `date`, `source`, `confirmed` and
`supersedes`. The field reports teams converging on machine-readable supersession "because
rebuilding the index doesn't resolve a semantic conflict"; mycelium's `[SUPERSEDED by: ...]` is
prose, so nothing can down-rank a dead claim or follow the chain.

**In the page, not one file per finding** — see the rejection below. `internal/memory/chunk.go`
already parses page frontmatter with `frontmatter()` and `scalar()`, so this is the same pattern
one level down rather than a new format.

### B2. `last_confirmed_at`

Recommended freshness metadata is `written_at`, `last_confirmed_at`, `expires_at`. Mycelium has the
first. Reinforce the second only when a memory proves correct during use, so a page verified
today and one written seven weeks ago and never rechecked stop looking identical.

### B3. A recency term in ranking

"Decay the score, not the data" — reversible, unlike deletion. Ranking is purely lexical today.

### B4. Let `[[wiki-links]]` contribute to ranking

The recommendation is fusing semantic similarity, BM25 and entity matching into one score. The
graph exists and is ignored.

### B5. A secrets clause in the storage gate

The recommended write gate has four conditions and mycelium's has three; "secrets never enter
memory" is missing. Flows already carry the rule.

### B6. An episodic-to-semantic consolidation stage

The largest item, and the one the field calls crucial. Mycelium captures episodically (`events/`,
sessions) and holds semantic knowledge (`memory/`) with nothing between them. The recommended
pattern is asynchronous: capture raw, then a background stage extracts candidates, consolidates
against existing pages and applies the write gate. The daemon is the obvious host. Today
consolidation happens only when an agent chooses to write a synthesis — one page in thirty.

## Decided, do not relitigate

- **Not CRDTs.** They guarantee convergence without conflicts, and that is the wrong goal here.
  Mycelium stores claims: a CRDT merges "X is true" and "X is false" into one page and calls it
  converged. The `[SUPERSEDED by: ...]` convention exists precisely because contradiction needs a
  human.
- **Not a vector database as the store.** Markdown stays canonical with search layered on top —
  the shape Manus, OpenClaw and Claude Code all converged on.
- **Never sync `.git`.** Documented corruption mode; `syncSkip` already prevents it.
- **Never write conflict markers into a page.** Obsidian Git's most common complaint is
  `<<<<<<<` landing inside a note. Merge cleanly or do not merge.
- **Agents know sync; agents never know git.** Sync is a domain concept an agent reasons about.
  Storage is not.
- **Three layers, never collapsed.** Findings hold what is true, `log.md` holds what changed and
  why, git holds every mechanical step. Agents search the first two. Collapsing the middle into
  the bottom is the mistake this roadmap made once already.
- **Retrieval stays chunk-level. Do not switch to whole-page retrieval.** Measured, n=5
  retrieved ([Chroma, Evaluating Chunking Strategies](https://www.trychroma.com/research/evaluating-chunking)):
  200-token chunks score 88.1% recall at 7.0% precision, 800-token chunks 87.9% recall at 1.4%.
  Equal odds of finding the answer, and five times as much irrelevant text dragged in with it —
  which costs real accuracy, because context degrades as it grows. `SearchChunks` already ranks
  `### finding` blocks rather than pages, and the same study found structure-aware boundaries
  beat fixed-size splitting, which is what a heading is. **This is already right; leave it.**
- **Do not build content filtering for memory safety.** Measured on memory poisoning
  ([arXiv 2606.04329](https://arxiv.org/html/2606.04329v1)): the best off-the-shelf detector
  reaches 67.67% TPR overall and **42.50% on weak-signal attacks**, and retraining moved PIGuard
  only from 38.33% to 47.67%. The reason is structural — a poisoned memory "carries no syntactic
  anomaly" and is "semantically indistinguishable from legitimate content", so no amount of
  reading the text separates the two. Attacks meanwhile succeed at 80–95% with a poison rate
  under 0.1%. The controls that survive this are **provenance** (already required by the wiki
  rules) and **revert** (A1), not a filter.
- **Not one finding per file.** The whole motivation was per-claim metadata, and B1 delivers that
  in place. Splitting would *cost* the thing that is measured: context is worth 35-49% of
  retrieval failures (Anthropic, Contextual Retrieval — baseline 5.7% failure at P@20 0.65,
  contextual embeddings 3.7% at 0.74, 49% fewer with contextual BM25), and `hybrid.go` already
  banks part of it because `chunkHeader()` enriches every chunk with its page's title and type.
  It would also add naming, frontmatter and placement ceremony to every agent write, against the
  agent-surface rule, and there is no write command to absorb that — `mycelium memory` has only
  `search` and `index`. And it cannot be settled cheaply: all 65 golden cases in
  `internal/memory/testdata/golden.json` name **page paths**, so a split invalidates the ground
  truth and turns the A/B into two unrelated measurements. Revisit only if a single page starts
  holding genuinely unrelated findings — which is "split this page in two", not "split every
  claim into a file".

## Not decided

### Does mycelium hold normative documents, and if so under what rules?

Mycelium's memory is **descriptive**: dated observations carrying provenance, written mostly by
agents, synced without ceremony. A standard is **normative**: it says "when a repo disagrees
with this file, the repo is wrong", and a change to it should land deliberately rather than
appear on every machine five minutes later.

Until 2026-08-22 the suite kept those apart by storing standards in a separate git repo. That
repo has been deleted and its documents are local-only, pending a decision here. Nothing in
mycelium's current model distinguishes the two kinds of page, which means an agent editing a
standard looks exactly like an agent filing a gotcha.

The shape that probably fits already exists in this codebase: **flow trust**. A flow arriving
over sync is refused until a human approves it on the machine that runs it, and an edited flow
re-enters `CHANGED`. A normative page wants the same treatment — it syncs freely, but a changed
one is not presented as authoritative until someone ratifies it.

This gates the migration of `CLI-STANDARD.md`, `DOCS-STANDARD.md` and `MIGRATIONS.md` into
mycelium, so it wants deciding before Track A finishes rather than after.

**Call it a bet, because it is one.** OWASP prescribes write-path provenance and source
isolation, and the memory-poisoning literature agrees — while stating plainly that these
"remain theoretical recommendations rather than tested implementations". Nobody has measured a
trust gate. It is the only remaining option once detection is ruled out, which makes it
reasonable, not proven.

**And the threat model here is milder than the papers'.** They assume an adversary. Mycelium's
realistic risk is an agent writing a wrong claim in good faith — on 2026-08-22 an agent asserted
"api.md is exempt from the docs ceiling" three times and built a design argument on it, from a
stale line in another repo's ROADMAP. That is textbook weak-signal poisoning with nobody
attacking. Against accidental poisoning provenance works far better than the adversarial numbers
suggest, because the agent is not hiding: it cites a source, and the source can be checked.
Checking the source is what caught that one.

## Exit criteria

- A page can be deleted and restored with `mycelium memory revert`, and the history names the
  machine that changed it.
- A mass deletion is refused without an explicit flag.
- `mycelium memory search` returns findings and `log.md` lines, and never a commit message.
- `log.md` still exists, and no agent writes to it more often than it does today.
- No agent-facing command, flag or output contains the word `git`.
- A search result shows how old its claim is.
