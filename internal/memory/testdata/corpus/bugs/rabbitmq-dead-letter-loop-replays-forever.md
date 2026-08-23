---
title: A dead letter exchange that routes back to its own queue spins forever
type: bug
tags: [rabbitmq, dead-letter, retry, x-death]
---

# Dead letter loops

### The broker counts nothing on your behalf
**Date**: 2026-06-14
**Source**: direct observation, nine thousand deliveries a second and no progress

A poison message rejected with `requeue=false` goes to the dead letter exchange
carrying its original routing key. The retry queue held it for thirty seconds
on a per message TTL and dead lettered it straight back to the work queue,
where it failed again. One message, two queues, a full CPU all night, and a
queue depth that never rose above one so no alert fired.

Each hop appends an entry to the `x-death` header. Read the count on the first
entry, and drop or park the message once it crosses a threshold:

```
x-death: [{count: 47, reason: rejected, queue: work, exchange: dlx}]
```

Rejecting a permanent failure with `requeue=true` builds the same loop without
an exchange in it. Park what exceeds the threshold in a queue nothing consumes
automatically, alert on that queue's depth, and treat a message arriving there
as a bug report rather than as retryable work.
