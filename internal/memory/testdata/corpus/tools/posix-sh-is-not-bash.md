---
title: /bin/sh is dash on Debian and it is not bash
type: tool
tags: [shell, posix, dash, portability]
---

# Scripts that only run under one shell

### The failure depends on the machine, not on the script
**Date**: 2026-05-27
**Source**: direct observation, `Syntax error: "(" unexpected` in a container build

A script with a `#!/bin/sh` line that uses bash features runs fine on a Mac,
where `sh` is bash in POSIX mode, and dies in a Debian-based image, where `sh`
is dash. Pipeline steps make it worse: several runners hand the command to
`sh -c`, so the line that worked in a terminal fails in the step that copied it.
The errors name the feature if you read them literally.

```
[[: not found                     # test brackets
source: not found                 # use . instead
set: Illegal option -o pipefail   # not POSIX
-e hello                          # echo -e printed the flag
```

The tools it calls have the same problem: `\s` is a GNU extension, so a portable
pattern spells the class out.

```sh
grep -n '[[:space:]]$' scripts/*.sh
```

Pick one of two answers and be consistent. Either write POSIX shell, with `.`
for sourcing, `[ ]` for tests and `printf` instead of `echo -e`, or declare
`#!/usr/bin/env bash` and check the image installs bash, which a slim base image
does not, along with the rest nobody packed either:
[[bugs/a-slim-container-carries-no-certificate-store]]. `shellcheck -s sh` finds
nearly all of it first, fast enough to run on every commit.
