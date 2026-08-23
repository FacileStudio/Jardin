---
title: Hash the lockfile into the cache key, use restore keys for near misses
type: convention
tags: [ci, cache, lockfile, build]
---

# Designing a cache key

### The key should change exactly when the contents should change
**Date**: 2026-04-08
**Source**: https://docs.github.com/en/actions/using-workflows/caching-dependencies-to-speed-up-workflows

A key naming the branch gives every new branch a cold start and never expires
when a dependency moves. A key that hashes the lockfile changes when, and only
when, the tree it describes changes. Add the runner OS and the toolchain
version to the front of it: a runner image upgrade can otherwise restore
binaries linked against a libc the new image does not have, and the failure is
far from the cache.

```yaml
key: ${{ runner.os }}-node20-${{ hashFiles('**/package-lock.json') }}
restore-keys: |
  ${{ runner.os }}-node20-
```

Restore keys are prefix matches tried in order and only on a miss, so the job
starts from last week's tree and the install tops up the difference instead of
downloading everything. The part people learn the hard way is that an entry is
immutable: saving again under a key that already exists is a silent no-op, so a
cache poisoned by a half-finished install stays poisoned until something
changes the key. That is another argument for deriving it from a file rather
than from a hand-written string.
