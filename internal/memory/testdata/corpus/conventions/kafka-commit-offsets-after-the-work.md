---
title: Commit the offset after the work, not after the poll
type: convention
tags: [kafka, offsets, delivery-semantics, consumer]
---

# Offset commit semantics

### Auto commit runs on a timer that knows nothing about your handler
**Date**: 2026-05-08
**Source**: https://kafka.apache.org/documentation/#consumerconfigs

With `enable.auto.commit` on, the client commits the offsets from the previous
poll every `auto.commit.interval.ms`, five seconds by default, on whichever
poll call comes next. Nothing consults the handler. A process killed after that
timer fires and before the last side effect is durable loses those records with
no error anywhere, which is at most once delivery shipped as a convenience
default.

Turn it off and commit once the effect is durable. That gives at least once
delivery, so the consumer has to tolerate seeing a record twice after a crash
or a rebalance, which is a smaller problem than silent loss and one the handler
can solve with a unique constraint or an upsert. Commit the offset of the next
record to read, `lastProcessed + 1`, per partition; committing the offset you
just processed replays it forever. Batch the commits rather than committing per
record, since a synchronous commit is a round trip to the coordinator.
