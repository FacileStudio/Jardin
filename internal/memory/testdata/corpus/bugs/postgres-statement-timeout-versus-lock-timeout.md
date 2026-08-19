---
title: statement_timeout does not bound time spent waiting for a lock
type: bug
tags: [postgres, timeout, lock, database]
---

# statement_timeout versus lock_timeout

### A query blocked on a lock is not a slow query
**Date**: 2026-04-30
**Source**: direct observation, migration wedged behind an idle transaction

`statement_timeout` bounds how long a statement runs. A statement waiting to
acquire a lock is running, so the timeout does eventually fire, but only after
the whole wait — which means an `ALTER TABLE` queued behind a long idle
transaction holds the queue for the full timeout and every reader behind it
blocks too.

```sql
SET lock_timeout = '3s';
SET statement_timeout = '30s';
```

`lock_timeout` bounds the wait specifically, so a migration gives up quickly
instead of forming a convoy. The connection that actually causes this is usually
idle in transaction rather than executing anything; `pg_stat_activity` shows the
state, and `idle_in_transaction_session_timeout` stops it recurring.
