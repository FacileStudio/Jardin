---
title: Postgres connection pool exhaustion under slow queries
type: bug
tags: [postgres, pool, timeout, database]
related: [read-your-write-lands-on-a-lagging-standby, postgres-statement-timeout-versus-lock-timeout]
---

# Postgres connection pool exhaustion

### A slow query holds its connection for the whole request, not the whole statement
**Date**: 2026-03-11
**Source**: direct observation, production incident

The pool was sized at twenty connections and the service ran fine at ten requests
per second until one report endpoint began taking eight seconds. Every in-flight
report held a pooled connection for its entire lifetime, so twenty concurrent
reports drained the pool and every unrelated endpoint blocked waiting to acquire
one. The database itself was almost idle: CPU under ten percent, no lock
contention, no slow log entries beyond the reports.

Raising the pool size moves the wall without removing it. The fix was a separate
pool for long-running analytical work, so a report can never starve the
transactional path:

```go
reports := pgxpool.Config{MaxConns: 4, MaxConnLifetime: time.Minute * 30}
```

The signal to watch is acquire wait time, not query duration. A pool that is
exhausted looks exactly like a fast database and a slow application.
