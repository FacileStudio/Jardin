# Roadmap

Written 2026-08-22, current as of 2026-08-24 and v0.23.0. A cold-start handoff: enough to pick up a track with no prior
conversation. **The executable form is `SPEC.md` — exact files, exact steps, exit criteria per
step.** This file holds the why and the ordering; the reasoning behind each item lives in the commit that
closes it, and the research behind both tracks is in
`~/.mycelium/memory/syntheses/agent-memory-practice-2026.md`.

## Where this is

Memory, rules, skills, flows, sessions and claims all ship. The `agents` adapter landed in
v0.19.0, so `~/.agents/AGENTS.md` and `~/.agents/skills/` are generated for every tool that
follows the AGENTS.md specification.

**Track A is done and shipped in v0.22.0**, released and deployed on 2026-08-23. **Track B is
done bar consolidation, shipped in v0.23.0 on 2026-08-24**: B4 closed the last ranking item and
the rules finally ask an agent to write the one metadata field that earns its keep, which leaves
**B6 as the only code item on this page**. A page can be
deleted and restored with `mycelium memory revert`, the history names the machine that changed it,
and no agent-facing output contains the word `git`.

**Memory has a history.** `internal/journal` versions `memory/`, `rules/`, `skills/`, `flows/`
and `extensions/` through go-git, committed inside the sync path and never blocking it. The
2026-08-19 accident, 246 pages gone with no way back, is now recoverable at any size rather than
only above the ten the A3 guard refuses. A machine with no history starts one on its next sync;
there is no migration step.

**The freshness signal reads what the corpus actually writes.** B1's metadata block turned out to
be written by nothing: 0 chunks of 476 carry an `id` or a `supersedes`, while 315 findings carry
the prose `**Date**:` line the writing convention mandates. B3 reads the block first and the
prose line second, which is the difference between a ranker that works on 329 chunks and one that
works on none. **See the correction under B1 below: the unit B1 chose was wrong for this wiki.**

## Shipped on 2026-08-24 (v0.23.0)

**B4 closed, the eval re-armed, and the standards imported.** B4's numbers and the two design
corrections that measuring forced are under `SPEC.md` step 11; the standards are at
`memory/standards/` in the wiki, not in this repository.

**The retrieval eval was re-armed** before any of that, because none of it could be graded
otherwise. It had been skipping since the 2026-08-19 reset and nobody
knew, so B3 and step 10 both shipped unmeasured. Four things changed:

- **Both evals graded `Search`**, the page-level path. Every agent-facing caller uses
  `SearchChunks`, where step 10's recency decay and struck-span dropping actually live. One-line
  fix; fixture MRR moved 0.974 to 0.990 and the floors held.
- **The live golden set left the repository.** Mycelium is public and the set is 76 plain-English
  descriptions of a private wiki's pages. It now lives at `~/.mycelium/eval/golden.json`, syncs like
  the wiki, and `loadGolden` skips when it is absent. Both it and `doctor` resolve through
  `config.DataDir()`, so they cannot read different trees.
- **`mycelium doctor` gained an `eval set` line** at the same 25% threshold the eval's own guard
  uses, so a stale set is visible instead of silent.
- **The fixture corpus went 30 to 70 pages**, with a 60-case hard set and a 10-case link set.
  `golden-crosslang.json` was retired: it proved the 2026-08-19 French to English conversion
  worked, a one-time measurement now enforced mechanically by the `wiki language` check.

**Read the numbers with the caveat.** Three of four sets sit at recall 1.000 and cannot show an
improvement. See the saturation risk in `SPEC.md`.

## Shipped on 2026-08-23 (v0.22.0)

- **The release.** Four platform tarballs, checksums, Homebrew tap at 0.22.0, server on the same
  commit, ruche on 0.22.0. **A push to main deploys production and does not wait for CI**; see
  AGENTS.md, which is where a contributor will look.
- **A1 and A2, the journal.** go-git over the authored tree, committed by the sync path.
  `Commit` stages four named roots rather than a caller's path list, because a caller can miss a
  file and a missed page is the accident this exists to prevent. Two commits per sync, not one:
  one before the reconcile for anything written since the last, one after for what moved. The
  first closes a window where a page an agent wrote an hour ago and a pull then deleted had never
  been recorded at all. Every failure is a warning, verified against a corrupt repository and a
  held lock.
- **The serialisation is a file lock.** `SPEC.md` read as an in-process race; the daemon runs
  `mycelium sync` as a child process, so the collision is between two processes and no mutex can
  see it.
- **`mycelium memory log`, `diff` and `revert`.** A revert snapshots what it is about to replace,
  so a wrong ref does not turn one lost page into two. A patch's header names the storage, so it
  is stripped and a test fails if it comes back.
- **B3, recency and supersession in ranking**, reshaped by two measurements. See B1 and B3 below.
- **A3, the bulk-delete guard** (v0.20.0), wider than this page describes it below. Both
  directions count against one limit, because a local wipe pushed up empties the copy every other
  machine pulls from, and that is the half no journal can undo yet.
- **A6, `.conflict` files out of `memory/`** (v0.21.0). The losing copy of an edit-vs-edit now
  mirrors the page under `~/.mycelium/.conflicts/<path>` and keeps its real extension. Both layouts
  are pruned and reported, because machines are still carrying the old sibling files.
- **A7, dot-directories pruned from the tree walk** (v0.21.0). `LocalTree` returns
  `filepath.SkipDir` rather than `nil`, so no sync descends into the `.git` A1 is about to
  create.
- **B1, per-finding metadata** (v0.20.0), parsed in `chunk.go` into fields on `Chunk`.
- **B5, the secrets clause** (v0.20.0), a fourth condition on the storage gate, in all 8 agent
  configs.
- Two review findings fixed alongside. The bulk-delete refusal prints entirely to stderr rather
  than splitting across two streams, and `doctor`'s `last sync` check fails past 24 hours instead
  of reporting the age and passing at any value. The second one mattered the moment a sync could
  stop and stay stopped.

## Start here

**1. The suite's standards are in the wiki. Done 2026-08-24**, at `memory/standards/{cli,docs,
migrations}.md`, with the `~/Nuage/Wiki` originals replaced by pointers.

**The premise this item was written on was wrong, and the correction is the lesson.** It said the
documents survived only in `~/backups/FacileStudio-Wiki-20260822.bundle`, verified 2026-08-23,
"nothing else on either machine has a copy". A full live copy sat in `~/Nuage/Wiki` the whole
time, reached by the symlink `~/Projects/Facile/Wiki` that the suite's own `CLAUDE.md` tells every
agent to read. The `$HOME` search that concluded otherwise missed it because `find` does not
follow symlinks by default. **A negative result from one search is not evidence of absence**, and
a finding whose entire value is "nothing else has a copy" has to be held to that.

So this was never a rescue from one bundle. It was two un-versioned copies diverging in opposite
directions: the Nuage copy held an uncommitted 2026-08-10 reversal of `CLI-STANDARD.md` §2.3 rule
7, the bundle held the 2026-08-22 revision of `DOCS-STANDARD.md` that the Nuage copy never got.
Each file was merged from whichever side was newer per hunk.

**2. The normative-documents question**, still open under "Not decided" below. It is what governs
those documents once they are in: a descriptive wiki page is a dated observation an agent files,
and a standard says "when a repo disagrees with this, the repo is wrong". The shape the roadmap
proposes is flow trust applied to pages, a normative page syncing freely but not counting as
authoritative until a human ratifies the change.

**3. The metadata block. Closed 2026-08-24, by asking for one field instead of five.** The rules
now tell an agent to stamp `<!-- confirmed: YYYY-MM-DD -->` under a finding it re-checks and finds
still true, and nothing else. `date` and `source` are not asked for, because the prose `**Date**:`
and `**Source**:` lines the convention already mandates cover them. `supersedes` is not asked for,
because B1's own correction says it is the wrong unit. `id` existed only to be its target.

`confirmed` earns its place by being the one field prose cannot express, and it closes B2. The
measured effect today is 0.058% — a five-day-old corpus has almost no decay to reverse. That is
the design working, not a disappointment: a claim at 180 days sits at 0.9250 and one stamp returns
it to 1.0000.

**4. B4, wiki links in ranking. Done 2026-08-24.** Link recall 0.000 → 1.000. See SPEC step 11 for
why the bounded multiplier this page recommended could not work, why half the graph turned out to
live in frontmatter, and why the live set's MRR fell 0.006 while recall held.

**5. B6, consolidation.** `SPEC.md` step 12. The largest item on this page, still last, and now
the only code item left on it.

Everything the journal opened was closed the same day. `extensions/` is versioned, `doctor`
gained a `history` check, and `node_modules/` is excluded from `syncSkip` on both sides so one
`bun install` in the data directory can never reach the server.

## Track A — give memory a history

The agent-facing surface must not change. Agents keep writing files and running `mycelium sync`;
they never learn that git exists. See `~/.mycelium/memory/conventions/mycelium-agent-surface.md`.

### A1. A go-git journal over the authored tree

**Done 2026-08-23**, as `internal/journal/`.

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

**Done 2026-08-23**, verified against a corrupt repository and a lock held by another process.
Both leave the sync at exit 0 with the pages through.

A corrupt repository, a held lock or a full disk must still let the memory write and the sync
succeed. The failure goes to the human, never to the agent — the same shape as `tools.Mycelium()`
in nacelle returning no tools when mycelium is absent rather than erroring.

### A3. A bulk-delete guard

**Done in v0.20.0, covering both directions.** See `SPEC.md` step 3.

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

**Done in v0.21.0.**

`internal/sync/conflict.go`

A conflict as an event is domain and `doctor` already reports it. A file called
`foo.md.conflict` sitting in the tree is storage leaking into the agent's view. Write it under
`~/.mycelium/.conflicts/` instead. Already excluded from sync, so it is local clutter only.

Once A1 lands, most of these stop being written at all: today's reconcile compares checksums and
has no common-ancestor **text**, so "edited on both sides" can only park a copy. Git objects
supply a merge base, so two agents appending different findings to one page merge cleanly.

### A7. Prune dot-directories in the tree walk

**Done in v0.21.0.**

`internal/sync/tree.go`

`LocalTree` skips dotfiles but does not return `filepath.SkipDir`, so it descends into `.git`
and stats every object before discarding each one. Every sync gets slower as history grows.

## Track B — make memory age honestly

**B1 and B2 are one change, not two.** Both want per-claim metadata that page-level frontmatter
cannot express, and both are delivered by the metadata block in `SPEC.md` step 1. B3 depends on
that block existing.

### B1. Per-finding metadata, in the page

**Done in v0.20.0, and it chose the wrong unit. Read this before building on it.**

Two things came out of measuring the corpus while B3 was built, and the second is a design
correction rather than a status update.

**Nothing writes the block.** 476 chunks in the live wiki, 0 with an `id`, 0 with a `supersedes`,
0 with a `confirmed`. 315 findings carry the prose `**Date**:` line instead, because that is what
the writing convention in the shared rules tells an agent to write and nothing tells it to write
the block. A format an agent is not instructed to produce is a format that does not exist. If the
block is meant to be used, the rules have to ask for it.

**Supersession here is a sentence, not a finding.** The convention is a `~~struck-through~~` claim
followed by `[SUPERSEDED by: ...]` and the correction, both inside one `###` block. `supersedes`
is a pointer between blocks, so it cannot express what the wiki actually does, and acting on it at
block level would demote the correction along with the claim. B3 handles the real convention by
dropping struck spans from the indexed text, and keeps `supersedesIDs` ready for the first finding
that writes a pointer.

The rest of the block is fine and `Date()` uses `confirmed` and `date` when they are there. The
lesson is narrower than the field's advice suggested: machine-readable supersession is worth
having, and the unit has to match the unit the humans and agents already correct.

An HTML-comment block under each `### heading` carrying `id`, `date`, `source`, `confirmed` and
`supersedes`. The field reports teams converging on machine-readable supersession "because
rebuilding the index doesn't resolve a semantic conflict"; mycelium's `[SUPERSEDED by: ...]` is
prose, so nothing can down-rank a dead claim or follow the chain.

**In the page, not one file per finding** — see the rejection below. `internal/memory/chunk.go`
already parses page frontmatter with `frontmatter()` and `scalar()`, so this is the same pattern
one level down rather than a new format.

### B2. `last_confirmed_at`

**Half done in v0.20.0, and still half done.** The `confirmed` field exists, parses, and is the
first thing `Chunk.Date()` reads. Nothing writes it, which is the same gap as B1: no rule asks an
agent to. Closing it is a rules change, not a code change.

Recommended freshness metadata is `written_at`, `last_confirmed_at`, `expires_at`. Mycelium has the
first. Reinforce the second only when a memory proves correct during use, so a page verified
today and one written seven weeks ago and never rechecked stop looking identical.

### B3. A recency term in ranking

**Done 2026-08-23.** "Decay the score, not the data", taken literally: nothing is deleted, the
decay is a multiplier, and a reconfirmed claim scores as new again.

Exponential, half-life 180 days, floored at 0.85, so the whole effect lives in a 15% band and can
only break ties. A settled convention must not lose to yesterday's note, which is the failure mode
the field warns about in both directions: too aggressive loses stable facts, too lax keeps stale
ones forever. On today's corpus the weights run 0.99743 to 1.00000, because the wiki is five days
old and age genuinely says nothing yet. That is the correct output, not a broken one, and it is
worth knowing before someone measures it and concludes recency does not work.

The part that pays today is supersession, and it needed a different mechanism than this page
assumed. See the correction under B1.

### B4. Let `[[wiki-links]]` contribute to ranking

The recommendation is fusing semantic similarity, BM25 and entity matching into one score. The
graph exists and is ignored.

**Done 2026-08-24**, as `internal/memory/links.go`. Recall on the link set went 0.000 → 1.000 and
the credit is additive, not the multiplier this page assumed; SPEC step 11 carries the numbers and
the two design corrections that measuring forced.

The three notes below were written for the worker who took it, and all three held:

- **Match targets against known page names, not against `[[...]]`.** Two false positives are
  planted in `testdata/corpus/tools/posix-sh-is-not-bash.md`: `[[:space:]]` in a grep fence, and
  bash `[[ ]]` test syntax. The live wiki has both classes for real.
- **Three `related:` spellings coexist** in the live wiki, plus body links written two ways. A
  resolver has to normalise `[[slug]]`, bare `[a, b]` and `[dir/slug.md]` to one thing.
  `normaliseLink` in `link_eval_test.go` does it by basename, which is why
  `TestCorpusBasenamesAreUnique` exists.
- **`frontmatter()` strips YAML before chunking**, so a `related:` link carries zero BM25 weight
  today while a body `[[link]]` adds its slug's words to that chunk. Whichever the signal reads,
  they are not equivalent inputs.

### B5. A secrets clause in the storage gate

**Done in v0.20.0.**

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

**All six met on 2026-08-23.** The last one was the closest to being missed: the ranking effect
is a fraction of a percent on a corpus this young, so "shows" was read as showing the date rather
than only scoring by it.

- A page can be deleted and restored with `mycelium memory revert`, and the history names the
  machine that changed it.
- A mass deletion is refused without an explicit flag.
- `mycelium memory search` returns findings and `log.md` lines, and never a commit message.
- `log.md` still exists, and no agent writes to it more often than it does today.
- No agent-facing command, flag or output contains the word `git`.
- A search result shows how old its claim is.
