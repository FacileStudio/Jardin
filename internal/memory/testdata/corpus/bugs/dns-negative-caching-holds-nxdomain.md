---
title: Negative DNS caching holds NXDOMAIN long after the record exists
type: bug
tags: [dns, cache, ttl, deploy]
---

# Negative caching holds a stale NXDOMAIN

### The TTL that governs a missing record is the SOA minimum, not the record's own
**Date**: 2026-03-04
**Source**: direct observation during a cutover, RFC 2308

A new hostname was published with a sixty second TTL and still failed to resolve
twenty minutes later on half the fleet. The resolvers were not caching the new
record, they were caching its absence: a lookup made before publication returned
NXDOMAIN, and how long that negative answer is held comes from the SOA record's
minimum field, which was set to an hour.

The practical rule is to lower the SOA minimum before a cutover, not after, and
to never resolve a name you are about to create. A single monitoring check that
polls the hostname early is enough to poison every resolver it speaks to for the
full negative TTL.
