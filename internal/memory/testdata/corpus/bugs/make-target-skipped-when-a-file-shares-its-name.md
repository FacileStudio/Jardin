---
title: A target with a matching file or directory is never rebuilt
type: bug
tags: [make, phony, build, timestamps]
---

# make decides a target is already up to date

### Every target name is first a filename
**Date**: 2026-03-28
**Source**: https://www.gnu.org/software/make/manual/make.html#Phony-Targets

A repository with a `test/` directory and a `test:` rule gets
`make: 'test' is up to date` and no tests. Make looked for something on disk
called `test`, found the directory, compared its modification time against the
prerequisites, and concluded there was nothing to do. The same happens for
`build`, `docs` and `install`, which is why every Makefile that survived
contact with users has this near the top:

```make
.PHONY: all test build clean install
```

Declaring the target phony also skips the implicit rule search, so make stops
trying to derive `test` from `test.c` and friends.

### A file written in the same second as its prerequisite looks stale
**Date**: 2026-03-28
**Source**: direct observation, generated file regenerating on every invocation

Make rebuilds when a prerequisite is newer, and on a filesystem storing whole
seconds a fast recipe finishes inside the same tick as its input. The
comparison then reads as not newer and the work repeats forever. GNU make uses
sub-second timestamps where the platform offers them; where it does not, the
fix is a stamp file the recipe touches once at the end.
