---
title: A consumer that stalls past max.poll.interval.ms rebalances the whole group
type: bug
tags: [kafka, consumer-group, rebalance, poll]
---

# Rebalance loops from slow processing

### The coordinator measures liveness in poll calls, not heartbeats
**Date**: 2026-04-11
**Source**: direct observation, a group that never finished its backlog

A batch of five hundred records took eleven minutes to handle. Heartbeats kept
arriving from the background thread, so the member looked alive, and the
coordinator evicted it anyway: progress is judged by the gap between successive
`poll` calls against `max.poll.interval.ms`, five minutes by default. The
partitions were revoked mid batch, the commit that followed failed, and the
reassignment handed the same records to another member that was equally slow.
The group spent the afternoon rebalancing and committed nothing.

```properties
max.poll.records=50
max.poll.interval.ms=900000
partition.assignment.strategy=org.apache.kafka.clients.consumer.CooperativeStickyAssignor
```

Shrinking the batch is the fix that holds. Raising the interval only widens the
window before a genuinely stuck worker is noticed, so do both and keep the
interval a small multiple of the worst observed batch. The cooperative assignor
matters for the second half of the problem: with the default eager strategy
every consumer in the group drops its partitions on each round, so one slow
member stops all of them.
