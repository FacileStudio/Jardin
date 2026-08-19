# `mycelium flow` — typed steps, the graph, and history

v0 ran shell steps in order. v2 let a step read what an earlier one produced.
This covers the three that complete the composition story: **typed steps** (v1),
**parallelism** (v3), and **querying history** (v4).

## v1 — typed steps

A step is either a shell command or a model, never both:

```yaml
steps:
  - name: check
    type: "@acme/http-check"
    with:
      url: https://example.test/health
      expect: 200
```

A **model** is TypeScript run by `bun`, living at
`~/.mycelium/extensions/models/<type>.ts` (the leading `@` is dropped, so
`@acme/http-check` is `acme/http-check.ts`). It answers two verbs:

- `describe` — print the schema as JSON on stdout
- `execute` — read `{"arguments": {...}, "env": {...}}` on stdin, print JSON on stdout

```ts
const schema = {
  type: "@acme/http-check",
  version: "1.0.0",
  arguments: { url: { type: "string", required: true } },
  outputs: ["status"],
};
```

mycelium never reads the TypeScript — only the schema. That is the whole point of
the split, and swamp puts it best: *a type is a class, a definition is an object
constructed from it.* An agent writing a flow needs the arguments, not the
implementation.

**Arguments are checked before anything runs.** Preflight resolves every model
the flow uses, asks each one to `describe` itself, and validates every step's
`with` against the answer. A missing required argument, a wrong type, a value
outside an `enum`, or an argument the model does not accept stops the run with
**no step executed** — not three steps in, after the side effects. Unknown
arguments are refused rather than ignored, for the same reason unknown YAML
fields are.

The model's stdout is the step's stdout, so its output chains through `needs`
like anything else. No new mechanism.

### Models are trusted per machine, like flows

A model is **code that arrives over sync**. Distributing prose an agent reads
and distributing code a machine runs are not the same risk, so a model is pinned
exactly like a flow:

```console
$ mycelium flow trust-model @acme/http-check
```

An unpinned model refuses to run; an edited one loses its pin and refuses again.
A type name is a path fragment and is checked for escaping — `../../.ssh/id_rsa`
does not resolve. `bun` is only required by flows that declare a `type`.

## v3 — the dependency graph

Steps form a graph, and steps with no edge between them run at the same time.

```yaml
steps:
  - name: lint
    depends_on: []
  - name: test
    depends_on: []
  - name: deploy
    depends_on: [lint, test]
```

**Parallelism is opt-in.** A step that declares no `depends_on` inherits the
dependency its position already implied — the step written above it. Every flow
written before this existed runs exactly as it did: one step at a time, in file
order. Declaring `depends_on`, *even as an empty list*, replaces that with the
edges asked for.

**Needing an output is a dependency.** A step with `needs: {V: build.stdout}`
waits for `build` whether or not `depends_on` repeats it, so the data and the
ordering cannot disagree. This retires v2's rule that a reference must point at
an earlier step: in a graph, order is a property of the edges. A reference is
checked for *existing*, and the graph is checked for cycles — the error names
the loop.

**Failure follows the semantics everybody already knows from GitHub Actions:** a
failed step blocks the steps that depend on it, and branches that never depended
on it run to completion. A timeout still stops the whole run.

Blocked steps are recorded, not dropped. Each is marked `skipped` and says which
dependency stopped it, which is the difference between an artifact that shows
what happened and one that stops mid-sentence.

`Parallel` caps how many steps run at once (default 4). Steps are shell
commands, so an unbounded fan-out is a fork bomb wearing a YAML hat.

## v4 — querying history

```console
$ mycelium flow query --status failed --since 7d
  deploy-check  failed     2026-08-19T09:14:02Z      1.2s  at smoke
  suite-check   failed     2026-08-18T17:40:11Z     14.8s  at test
```

Flags: `--status`, `--since` (`7d`, `24h`, `all`), `--flow`, `--limit`, `--json`.

It reads *history*, not the flow list, so a flow you deleted still answers for
what it did while it existed. Runs sort by the parsed start time, never by file
name — run IDs are RFC3339Nano, which drops trailing zeros and does not sort
lexicographically. The failed-steps column names the steps that actually broke
and omits the ones that only went down with them; a list of casualties buries
the cause.

`--since` reuses the same parser as `mycelium stats`. A window parser that
disagrees with the one next to it is worse than no window parser.

### Ephemeral output

```yaml
  - name: mint
    run: ./issue-token.sh
    ephemeral: true
```

The value still reaches the steps that need it and never lands on disk. Chaining
already carries the raw value in memory and writes only a redacted copy to the
artifact, so this is the natural end of that path rather than a new mechanism.
The step records *that* output was withheld, so the artifact does not read as a
step that printed nothing.

**Not built:** the rest of the lifetime taxonomy (job, workflow, duration,
infinite). `RunRetention` already bounds history, and retention by age is worth
having the day something needs it — it is also what would force `flow query`
onto an index, since it reads every artifact today and that is affordable only
because the cap exists.

## Compatibility

`depends_on`, `ephemeral`, `type` and `with` are new fields, and flow files are
parsed with unknown fields rejected — that is what makes a typo fail loudly. A
machine running an older mycelium refuses a flow that uses them rather than
running it wrong. **Update mycelium on every machine that runs the flow before
adding them.**
