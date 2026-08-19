# `jardin flow` — v2: data chaining

v0 ran steps in order and threw their output away. v2 lets a step read what an
earlier step produced, which is the brick that makes multi-step work composable.
Nothing else about the execution model changes: steps are still sequential,
still trusted per machine, still run through `sh -c`.

## File format

```yaml
name: release
steps:
  - name: version
    run: git describe --tags --always

  - name: notify
    needs:
      VERSION: version.stdout
      CODE: version.exit_code
    run: curl -fsS -d "shipping $VERSION" https://hooks.example/notify
```

`needs` maps an **environment variable name** to a **`<step>.<field>` reference**.

- Fields are `stdout`, `stderr` and `exit_code`. There is nothing else, because
  there is nothing else a shell step reliably produces.
- The reference is cut at the **last** dot, so a step whose own name contains
  one still resolves.
- Values arrive as ordinary environment variables. Read them as `"$VERSION"`.

There is no expression language, and deliberately so. The roadmap left room for
CEL "if an expression is needed"; a plain name-to-reference mapping covers
chaining without adding a syntax anyone has to learn, and CEL can still be added
later for the query layer without changing this format.

## What is refused, and when

Everything a flow file can get wrong is caught by `Parse`, so it fails before a
single step runs — the same rule v1 sets for typed arguments:

| Refused | Message names |
|---|---|
| A reference to a later step | `needs "x", which does not run before it` |
| A reference to a step that does not exist | the same |
| A step needing its own output | `needs its own output` |
| A field that is not stdout/stderr/exit_code | `exposes only stdout, stderr and exit_code` |
| A name set in both `env` and `needs` | `sets V in both env and needs` |
| A name a shell cannot read back as `$NAME` | `not a usable environment variable name` |
| Binding `JARDIN_TOKEN` | `may not bind JARDIN_TOKEN` |

Forward references are refused rather than deferred. Steps run in order, so a
later step's output cannot exist by the time an earlier one starts; accepting
the syntax and failing at run time would only move the error further from the
typo. When a DAG lands (v3), this is the one rule that has to be relaxed.

Two more are only knowable at run time, and fail the run rather than passing on
a value that is quietly wrong:

- **A truncated stream.** Capture stops at `MaxStreamBytes` (1 MB). Handing on
  the first megabyte of something as if it were the whole value corrupts the run
  silently, so the consuming step never starts.
- **An oversized or binary value.** See below.

A run stopped this way gets status `unresolved`, not `failed`, and the step that
never ran is marked `not_started` in the artifact. A step that ran and returned
non-zero and a step that never ran are different events; one bucket for both
sends the reader to the wrong place.

`allow_failure` does not cover either. A reference that cannot be satisfied is a
defect in the flow, not a step that was allowed to fail.

## Values are data, never program text

No value is ever spliced into the string handed to `sh`. This is the constraint
v0 was designed around and v2 keeps: the value travels in the child's
environment, and `run` refers to it by name.

This is also the mitigation GitHub documents for its own script-injection class
— route the untrusted value through an intermediate environment variable and
read it with native shell syntax, rather than interpolating it into the command.
A value of `; rm -rf /` reaches the step as those eleven characters and nothing
else; there is a test that proves it.

Two properties worth stating because they are easy to assume away:

- **Even unquoted, a chained value is not re-scanned for command substitution.**
  `printf '%s' $VALUE` word-splits the value but never executes it. Quote your
  references anyway — word-splitting is still the flow author's problem.
- **A producing step cannot invent a binding.** Bindings are declared by the
  *consumer*, in `needs`. This is why `jardin flow` has no equivalent of the
  `::set-output` hole GitHub Actions spent a deprecation cycle closing: nothing
  parses a step's stdout looking for directives.

## Size and encoding limits

A chained value is capped at `MaxValueBytes` (64 KB). The wall is the operating
system, not us: Linux refuses a single environment entry over `MAX_ARG_STRLEN`
(128 KB) and `execve` fails with `E2BIG`, whose message names the shell rather
than the flow. Measured on ruche — a 127 KB value passes, 129 KB gives
`fork/exec /usr/bin/sh: argument list too long`. Argo Workflows ships this exact
crash for this exact reason; Tekton dodges it by capping results at 4 KB.

Refusing at 64 KB — half the wall — leaves room for several needs plus the
inherited environment, and produces an error that names the step and suggests
the fix: **write it to a file and pass the path**. That is the right shape for
anything large anyway.

A value containing a **NUL byte** is refused for the same reason: Go answers
`exec: environment variable contains NUL`, which says nothing about which step
produced it.

One step's values are capped in total at `MaxTotalValueBytes` (256 KB), because
the per-value limit is not enough on its own: `ARG_MAX` bounds the whole
environment (~1 MB on macOS), so a handful of maximum-size values would fail at
exec with that same shell-blaming message.

## Trailing newlines are trimmed

`stdout` and `stderr` have trailing line endings stripped, the way `$(...)` does
in a shell. `git describe` therefore yields `v0.12.0`, not `v0.12.0\n`. Nothing
else about the value is touched — not leading whitespace, not internal newlines.

One deliberate difference from `$(...)`: trailing `\r` is stripped too, not only
`\n`, so a value produced by a CRLF-emitting tool arrives clean.

## Secrets

The value a step receives is the **raw** one; the value written to the artifact
is redacted. Redaction is about the record, not the data flow — a step bound to
`API_TOKEN` gets the real token and the artifact shows `***`. Swamp reached the
same conclusion from the other direction: sensitive fields are "stored but
redacted from CLI output and logs", so downstream access keeps working.

Guessing from the name is still the fallback, but a step no longer has to rely
on it: `secret:` lets a step declare which of the values it consumes are
sensitive, whatever they are called, and `ephemeral:` does the same for a
value it produces. Declared names are checked in `Parse`, join the same
redaction set as the ephemeral values, and outrank the length floor that
guards the guesses — the way swamp's output specs mark a field sensitive
instead of pattern-matching its name.

```yaml
steps:
  - name: publish
    secret: [GH_PAT]
    run: ./publish.sh
```

## The artifact

Each step records what it received, redacted:

```json
{
  "name": "notify",
  "exit_code": 0,
  "resolved": { "CODE": "0", "VERSION": "v0.12.0" }
}
```

`jardin flow show` prints those lines under the step, sorted by name, so a run
can be read back without guessing what `$VERSION` held at the time.

## Compatibility

`needs` is a new field, and flow files are parsed with unknown fields rejected —
that is what makes a typo fail loudly instead of being ignored. A machine
running a jardin older than this release will therefore refuse a flow that uses
`needs`, reporting it as an invalid field rather than running it wrong. One bad
flow does not hide the others in `flow list`, so the failure is legible and
local: **update jardin on every machine that runs the flow before adding
`needs` to it.**

The trust store is unaffected. Checksums are taken over the file's bytes, so
adding `needs` to an existing flow shows up as `CHANGED` and needs re-pinning,
exactly like any other edit.
