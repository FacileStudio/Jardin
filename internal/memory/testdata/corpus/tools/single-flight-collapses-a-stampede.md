---
title: One in-flight lookup, every later caller waits on its result
type: tool
tags: [go, concurrency, cache, deduplication]
---

# Collapsing duplicate work

### A cold entry is asked for by everybody at the same instant
**Date**: 2026-06-28
**Source**: https://pkg.go.dev/golang.org/x/sync/singleflight

When a hot cache entry expires, every handler that wanted it misses at that
instant and each one goes to the origin. The origin sees a thousand identical
lookups for a value absent for a millisecond, and the recovery costs more than
the miss did. Capacity does not help: the spike was created by the expiry.

`singleflight` fixes the shape rather than the timing. The group keys work by a
string: whichever goroutine arrives while nothing is running does the lookup and
everybody arriving during it blocks on that one result.

```go
v, err, _ := group.Do(k, func() (any, error) { return fetch(ctx, k) })
```

Three edges decide whether this helps or hurts. The result is shared and so is
the failure, so one unlucky timeout is handed to every waiter at once; `DoChan`
with a select on the caller's own deadline keeps a slow leader from pinning
unrelated work. The key has to name the work exactly, or callers wanting
different things queue behind each other. And the collapsing must sit below any
retry loop, since a retry above it is an arrival the group has forgotten.

The pattern generalises past caches: anything that must happen once per process
rather than once per caller wants it, from opening a handle lazily to warming a
table that costs a round trip.
