---
title: With no prefetch limit one consumer drains the queue into its own memory
type: bug
tags: [rabbitmq, prefetch, qos, consumers]
---

# Unacked messages pile up on one consumer

### basic.qos is unlimited until somebody sets it
**Date**: 2026-03-05
**Source**: direct observation, three idle workers and one busy one

Adding workers changed nothing. The management view explained why: ready 0,
unacked 40000, and a single consumer holding all of it. The broker pushes
messages as fast as the socket accepts them to whichever consumer is available,
so the first one to connect took the whole backlog into its client buffer and
the others found an empty queue. When that process was restarted the forty
thousand were redelivered in one burst to the next consumer, which repeated the
pattern.

```python
channel.basic_qos(prefetch_count=20)
```

The limit counts messages delivered but not yet acknowledged, per channel. One
suits long jobs where an even spread matters more than round trips, twenty to a
couple of hundred suits short jobs where the round trip dominates. It has no
effect at all when the consumer acknowledges automatically, because then
nothing is ever outstanding to count.
