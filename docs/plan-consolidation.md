# Plan — SPEC step 12: episodic-to-semantic consolidation

Status: proposed 2026-08-24. Implements ROADMAP B6 / SPEC step 12 with two
design additions ratified in planning conversation: hybrid heuristic+local-LLM
extraction, and supersede-candidates as a first-class output.

## Goal

A daemon stage reads recent episodes from `~/.mycelium/events/`, extracts
candidate findings, consolidates them against existing memory pages, and
writes passing candidates directly into `memory/` (create or supersede) —
fully automatic, no user interaction, no cloud tokens.

## Why (evidence) — one line

Mycelium captures episodically (`events/pi/*.jsonl`, 11k lines/month) and holds
semantic knowledge (`memory/`) with nothing between them; consolidation today
happens only when an agent chooses to write a synthesis — one page in thirty
(ROADMAP §B6).

## Approach

New `internal/consolidate/` package hosted as one stage inside the existing
daemon tick (`internal/daemon.Run()` already chains sessions-scan → usage →
sync → install; consolidation slots after sync, before install-gating).
Extraction is **hybrid and lazy**: cheap deterministic heuristics propose
candidates from episode windows; a **local Ollama model** (reusing the
existing `internal/memory/ollama.go` client pattern) judges durability only
for heuristic hits — so zero API tokens are spent, and an offline machine
fails open (heuristics-only mode). Accepted candidates go through the same
storage gate agents follow, then are written directly to `memory/` using the
wiki's own prose conventions (`### finding` + `**Date**:`/`**Source**:`
block, or `~~struck~~ [SUPERSEDED by: ...]` for contradictions). A watermark
cursor makes runs idempotent; episodes are never reprocessed.

Field alignment: RecMem (lazy consolidation), Memini (heuristic propose +
LLM "durability judge"), Mem0 issue tracker (ADD-only rots retrieval →
supersession must be first-class), storage gate ≈ community write-gate
consensus.

## Steps (ordered)

1. **`internal/consolidate/source.go`** (new) — pluggable episode reader.
   Interface `Source { Name() string; Since(watermark time.Time) ([]Episode,
   error) }`. First implementation reads `events/<agent>/*.jsonl` (pi today;
   claude/codex/others land as files under their own dir later — the reader
   keys on directory, not on the pi schema; unknown JSONL shapes yield
   episodes with text fields best-effort extracted from any `"message"`,
   `"content"` or `"text"` string values). Exit: table test over a fixture
   JSONL returns episodes; a foreign-schema JSONL still yields text.

2. **`internal/consolidate/heuristic.go`** (new) — candidate proposer.
   Deterministic patterns over episode text: error→fix pairs (error message
   followed within N lines by resolution), explicit gotcha markers ("gotcha",
   "turns out", "the fix was", "note that"), repeated failures across
   sessions (same error ≥2 distinct timestamps). Output:
   `Candidate{Text, EpisodeRefs, Kind}`. Exit: table tests, one hit per
   pattern class, zero hits on noise fixture. `[filet]`

3. **`internal/consolidate/judge.go`** (new) — local durability judge.
   Prompt asks a small local model (configurable, default e.g.
   `llama3.2:3b`) one yes/no question: "will this be useful in 30 days?"
   Reuses the HTTP-client shape of `internal/memory/ollama.go`. Fails open:
   Ollama unreachable or unconfigured ⇒ judge returns `accept` with
   `JudgedBy: "heuristic-fallback"` (gate still applies downstream). Config
   via existing `internal/config` (new key `consolidate.judge_model`, empty =
   fallback mode). Exit: test with httptest server for both accept and
   unreachable paths. `[filet]`

4. **`internal/consolidate/gate.go`** (new) — the storage gate, executable.
   Four checks mirroring `~/.mycelium/rules/20-memory.md`: changes future
   behavior / non-obvious / grounded (has episode refs = provenance) / no
   secrets (reuse the secrets-clause patterns from `internal/flow`). Returns
   typed `Rejection{Reason}` — never just a bool. Exit: each rule has a
   pass and fail case; secret detection shares test vectors with flow's.

5. **`internal/consolidate/dedupe.go`** (new) — consolidation against
   existing pages. For each gated candidate: embed it via the existing
   `internal/memory` embedding path, search the wiki (reuse
   `SearchChunks`/hybrid ranking). Three outcomes: `NOOP` (top match above
   near-duplicate threshold and not contradicted), `CREATE` (no strong
   match), `SUPERSEDE` (strong match whose claim the candidate contradicts
   — judged by the local model comparing the two texts, same fails-open
   policy). Thresholds as package constants, tuned against the eval corpus.
   Exit: fixture wiki + fixture candidates produce one of each outcome.
   `[filet]`

6. **`internal/consolidate/write.go`** (new) — direct wiki writes following
   the prose conventions exactly. CREATE appends a `### finding` block with
   `**Date**`/`**Source**` (source = episode refs, machine + timestamp) to
   the right page (dedupe's match target, or a new page under the matching
   top-level dir when none). SUPERSEDE strikes the old claim in place and
   writes the correction beneath it — the exact `[SUPERSEDED by: ...]`
   convention, never a delete. Writes go through the same page-writing code
   path agents use (check `internal/memory` for an existing writer; if
   writes are currently raw-file by agents only, add the smallest writer
   here). Exit: golden-file test of before/after page content for both
   operations. `[filet]`

7. **`internal/consolidate/cursor.go`** (new) — idempotency. Watermark file
   under `config.DataDir()` (e.g. `.consolidate-cursor.json`: per-source
   last-processed timestamp + hash of last line). Crash-safe: advance only
   after successful write phase. No manual override flag — reprocessing is
   what `sync --force` culture exists to avoid; deleting the cursor file is
   the escape hatch (documented, agent-discouraged). Exit: interrupted-run
   test reprocesses nothing on second run.

8. **`internal/daemon/daemon.go`** — wire the stage. After sync, run
   consolidate with a guard: skip when events unchanged since watermark
   (cheap stat check), rate-limit to at most once per hour regardless of
   tick frequency. Log refusals (with reasons) to the daemon log path.
   Exit: daemon test with fake clock runs the stage once, then skips on the
   immediate second tick.

9. **Doctor + docs.** `cmd/doctor.go` gains a `consolidate` line (watermark
   age, last run outcome, counts created/superseded/dropped-with-reason).
   `docs/configuration.md` gains the `consolidate.*` config section;
   `docs/architecture.md` gains the pipeline description. CHANGELOG entry.
   Exit: doctor shows the line on a machine with events; docs match flags
   actually implemented.

## Files to Modify / New

- `internal/consolidate/source.go` — new, episode readers (pluggable per-harness)
- `internal/consolidate/heuristic.go` — new, candidate proposer `[filet]`
- `internal/consolidate/judge.go` — new, local Ollama durability judge `[filet]`
- `internal/consolidate/gate.go` — new, executable storage gate `[filet]`
- `internal/consolidate/dedupe.go` — new, NOOP/CREATE/SUPERSEDE decision `[filet]`
- `internal/consolidate/write.go` — new, convention-shaped wiki writes `[filet]`
- `internal/consolidate/cursor.go` — new, watermark idempotency
- `internal/config/config.go` — modify: `consolidate.judge_model`,
  `consolidate.enabled` (default true)
- `internal/daemon/daemon.go` — modify: stage wiring + hourly rate-limit
- `cmd/doctor.go` — modify: `consolidate` health line
- `docs/configuration.md`, `docs/architecture.md` — modify
- `CHANGELOG.md` — modify

No DB migrations (file-based state), no new Go deps beyond what
`internal/memory/ollama.go` already uses (stdlib net/http).

## Exit criteria (SPEC step 12, verbatim plus)

- A daemon run over seeded events produces a candidate that passes the gate
  and lands as a well-formed wiki write.
- A failing candidate is dropped with a reason visible in the daemon log and
  doctor output.
- Second run over identical input writes nothing (idempotent).
- Zero network calls outside localhost (Ollama only); works fully offline in
  heuristic-fallback mode.
- `go test ./...` green, `filet check .` exit 0, every new file under the
  complexity caps.
- No agent-facing output contains the word "git".

## Risks / unknowns

- **Supersede misfires are the dangerous case** — auto-striking a correct
  claim on weak evidence poisons the wiki silently. Mitigation: SUPERSEDE
  requires both high embedding similarity AND judge-model agreement AND the
  contradiction being dated newer than the struck claim's `**Date**:`. When
  in doubt, NOOP. Tune on the eval corpus before enabling by default.
- **Write conflicts with concurrent agent edits** — pages may change between
  dedupe-read and write. Mitigation: read-modify-write per page kept short;
  the journal makes any clobber revertable, and the sync conflict machinery
  already catches divergence.
- **Local model quality varies wildly** — a bad small model may rubber-stamp
  everything. Mitigation: judge prompt is binary and logged with verdicts;
  doctor surfaces accept-rate, so a degenerate 100%-accept model is visible.
- **Events schema drift across harnesses** — the shapeless reader is
  best-effort by design; per-harness adapters come later if quality demands.

## Skip (YAGNI)

- Cloud LLM judging (tokens, keys, latency — local covers it)
- Review queue / draft area (user decided: fully automatic)
- Decay/TTL auto-deletion (violates audit-trail design; supersede-in-place
  is the forgetting mechanism)
- Per-harness bespoke parsers beyond pi (add when a second harness's events
  actually exist and the generic reader misses findings)
- Embedding-model choice UI (reuse whatever `internal/memory` already uses)
- Cross-machine consolidation coordination (each machine consolidates its
  own episodes; sync merges results — conflicts already handled upstream)

---

Checked against: suite conventions via `~/.mycelium/memory/conventions/`
(mycelium-agent-surface, facile-cli-standard), SPEC.md steps 1–11 outputs
(chunk parsing, freshness, hybrid ranking, ollama client), ROADMAP §B6 +
exit criteria, repo AGENTS/filet caps, storage-gate rules in
`~/.mycelium/rules/20-memory.md`.
