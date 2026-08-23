---
title: Kafka orders records inside one partition and nowhere else
type: bug
tags: [kafka, partitioning, ordering, producer]
---

# Per entity ordering needs a key

### A record with no key is spread over every partition
**Date**: 2026-02-19
**Source**: direct observation, an update applied before its create

The producer published account events with a null key, so the partitioner
scattered them over twelve partitions and twelve consumers handled them at
once. The update for one account was processed before the create it depended
on, and the handler wrote a row with half its fields empty. The broker never
promised more than it delivered: total order exists within a partition, never
across a topic.

Setting the key to the account id routes every record for that account through
`hash(key) % partitions`, which is one partition and therefore one consumer at
a time. Two things still break it afterwards. Adding partitions changes the
modulus, so a key moves and its new records can overtake the ones already
queued behind the old assignment. And a producer with retries enabled and
`max.in.flight.requests.per.connection` above one can reorder a failed batch
against the batch behind it unless `enable.idempotence` is on.
