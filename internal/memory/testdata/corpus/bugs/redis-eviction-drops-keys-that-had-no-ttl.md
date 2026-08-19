---
title: Redis eviction removes keys you never marked expirable
type: bug
tags: [redis, eviction, memory, cache]
---

# Eviction under memory pressure

### allkeys-lru does not care whether you set a TTL
**Date**: 2026-05-22
**Source**: direct observation, session store emptying at random

Sessions were written without an expiry on the assumption that a key with no TTL
is permanent. Under `maxmemory-policy allkeys-lru` that assumption is false:
once the instance reaches `maxmemory`, the least recently used key is evicted
whatever its expiry, and a session nobody has touched for an hour is exactly the
least recently used key.

The policies that respect the distinction are `volatile-lru` and its siblings,
which only consider keys that carry a TTL and return an out-of-memory error when
none are left. That error is louder and better than silent data loss. Mixing
cache and durable state in one instance is the underlying mistake; the policy
choice only decides which way it fails.
