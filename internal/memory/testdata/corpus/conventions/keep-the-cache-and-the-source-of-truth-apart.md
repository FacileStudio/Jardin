---
title: A cache and a store of record do not belong in one process
type: convention
tags: [redis, cache, durability, architecture]
---

# One process, one durability class

### Sharing an instance lets the memory policy decide what you can afford to lose
**Date**: 2026-08-09
**Source**: team decision, taken after the second incident of this shape

A cache is defined by being safe to throw away. A store of record is defined by
not being. Put both behind one Redis and the eviction policy becomes the arbiter
of which of your data is expendable, and its answer is whichever entry was least
recently read, which has nothing to do with what the business can afford to lose.

Two processes cost one more port and settle several arguments at once. The cache
gets a memory ceiling, an eviction policy, no persistence, and an application
that survives it being empty at any moment, which you prove by flushing it in
staging during a load run rather than by asserting it in a design review. The
store of record gets persistence on, a restore somebody has actually performed,
an alarm on the backup, and no policy that lets anything be discarded quietly.

The split also separates the alerts, which is where the value shows up at three
in the morning. On the cache, the number to watch is how often a lookup
succeeds, and a drop in it is a capacity conversation for next week. On the
store, the number to watch is headroom, and approaching the ceiling is an
incident tonight, because the correct behaviour there is to refuse new data
loudly rather than to discard old data in silence.

The migration is dull and worth doing before you need it: point the cache client
at the new address, let the old entries age out, and leave the durable data
exactly where it is. Doing it afterwards means doing it during an audit.
