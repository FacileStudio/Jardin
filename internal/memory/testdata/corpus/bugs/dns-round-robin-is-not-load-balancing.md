---
title: Multiple A records do not balance load
type: bug
tags: [dns, resolver, ttl, balancing]
---

# Round robin DNS is not a load balancer

### Clients cache one answer and keep using it
**Date**: 2026-01-19
**Source**: direct observation, one backend at ninety percent while two idled

Publishing three A records distributes which address a resolver hands out, not
where traffic goes. A client resolves once, caches the answer for the TTL, and
frequently pins the first address in the list for the life of the process — JVM
and Go HTTP clients both keep connections open far past any TTL. The result is
traffic clumped on whichever address happened to be returned first to the
busiest client.

There is also no health signal: removing a failed address depends on every
resolver and every client honouring a TTL they are free to ignore. Round robin
is acceptable for spreading cold starts and useless for failover, which needs
something that terminates connections and knows whether a backend is alive.
