---
title: Pipelining is about round trips, MULTI is about atomicity
type: tool
tags: [redis, pipeline, transaction, latency]
---

# Pipelining versus transactions

### They solve different problems and are constantly confused
**Date**: 2026-05-28
**Source**: https://redis.io/docs/manual/pipelining/

Pipelining sends many commands without waiting for each reply, turning a hundred
round trips into one. It gives no atomicity whatever: other clients interleave
freely, and a command failing does not stop the rest.

`MULTI`/`EXEC` gives atomicity — the block runs with nothing interleaved — but
the client still waits for the queueing replies unless the whole block is also
pipelined, so it is not itself a latency optimisation. The two compose: pipeline
a `MULTI` block to get both.

Neither gives rollback. A command that fails at execution time inside `MULTI`
leaves earlier commands applied, because Redis has nothing to undo them with.
Where real conditional atomicity is needed, `WATCH` with a retry loop, or a Lua
script, is the tool.
