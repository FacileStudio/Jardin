---
title: An indexed document is not searchable until the next refresh
type: bug
tags: [elasticsearch, refresh, near-real-time, testing]
---

# Index then search returns nothing

### Fetching by id reads the translog, searching does not
**Date**: 2026-02-06
**Source**: direct observation, a test suite that failed once in five runs

The write returned 201, the query that followed returned zero hits, and the
same query a second later returned one. A document becomes visible to search
only when the shard refreshes, every `index.refresh_interval`, one second by
default and often thirty on a cluster tuned for bulk loading. Reading that same
document by id works immediately, which is what makes the failure look
impossible for the first hour.

`?refresh=wait_for` on the write blocks until the next scheduled refresh, and
it is what a test or a create-then-redirect flow wants. `?refresh=true` forces
a refresh on the spot, which produces a tiny segment per call and wrecks
indexing throughput along with the merge policy. Sleeping in the test is the
version that passes on a laptop and fails on the build machine, since the
interval is a cluster setting nobody checked. Wait on the condition rather than
on the clock, as in [[wait-for-a-condition-with-a-deadline]].
