---
title: SQLITE_BUSY under WAL is a writer queue, not corruption
type: bug
tags: [sqlite, wal, lock, database]
---

# database is locked, under WAL

### WAL removes reader-writer contention, not writer-writer contention
**Date**: 2026-04-08
**Source**: direct observation, https://sqlite.org/wal.html

Turning on write-ahead logging lets readers proceed while a write is in flight,
which is why it is the first thing anyone enables. It does not allow two
concurrent writers: SQLite still takes a single write lock, and a second writer
gets `SQLITE_BUSY` immediately unless a busy timeout tells it to wait.

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
```

The timeout is per connection and defaults to zero, so every connection in a
pool needs it set. A long-running read transaction also prevents the WAL from
being checkpointed, which grows the file without bound and looks like a disk
leak; the fix there is shorter read transactions, not a bigger disk.
