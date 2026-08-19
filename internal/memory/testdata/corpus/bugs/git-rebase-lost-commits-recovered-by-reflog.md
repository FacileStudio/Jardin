---
title: Commits lost to a bad rebase are still in the reflog
type: bug
tags: [git, rebase, reflog, recovery]
---

# Recovering commits after a rebase

### Nothing is deleted until garbage collection runs
**Date**: 2026-02-27
**Source**: direct observation, panicked colleague

An interactive rebase that drops the wrong line, a `reset --hard` on the wrong
branch, an abandoned conflict resolution: in every case the old commits still
exist as objects, unreferenced by any branch but recorded in the reflog for
ninety days by default.

```sh
git reflog --date=iso
git branch rescue HEAD@{7}
```

The reflog is per repository and per clone, never pushed, so this only works on
the machine where the mistake happened. Recovering someone else's lost work
means finding the machine that did it. `git fsck --lost-found` finds dangling
objects the reflog has already expired, which is the last resort when the ninety
days have passed.
