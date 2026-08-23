---
title: Each Make recipe line runs in a separate shell
type: bug
tags: [make, python, venv, shell]
---

# Activating a virtualenv in a Makefile does nothing

### The activation dies with the line that performed it
**Date**: 2026-06-09
**Source**: direct observation, `pytest: command not found` under make

Make hands each line of a recipe to its own `$(SHELL)` process. Whatever the
first line exported is gone by the second, so this runs the system interpreter
or none at all:

```make
test:
	source .venv/bin/activate
	pytest -q
```

Two fixes work and one is better. Joining the lines with `&&`, or declaring
`.ONESHELL:`, keeps a single process alive for the whole recipe. Calling the
interpreter by path instead, as `.venv/bin/pytest -q`, needs no shell feature at
all and does exactly what activation does: it puts that directory first on
`PATH` and sets `VIRTUAL_ENV`. The path form also survives `SHELL` being dash,
where `source` is not a builtin and the recipe fails with a different error.
