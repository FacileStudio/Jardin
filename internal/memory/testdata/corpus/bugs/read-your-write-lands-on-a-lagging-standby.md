---
title: A read that follows its own write can land on a standby that has not caught up
type: bug
tags: [postgres, replication, standby, consistency]
---

# Reads on a standby lag the write

### Streaming replication is asynchronous, so the row is not there yet
**Date**: 2026-07-24
**Source**: direct observation, a form that redirected to a page showing the old value

Sending heavy analytical work to a standby is the right shape and it quietly
changes what the application may assume. A commit on the primary returns as soon
as its own write-ahead log is durable. The standby replays that log a few
milliseconds later, or minutes later while it is busy with a bulk load, and a
read sent there in between sees the state as it was before.

The failure is intermittent and always looks like caching. A create followed by a
redirect to the new record renders a 404, a save and re-render shows the previous
value, and a job that writes then re-reads sends a message with a field missing.
Under light traffic the delay is a millisecond and nothing shows; under the load
that made the split worthwhile, it is plainly visible.

Send the read to the primary when it belongs to the same logical operation as the
write, decided per session rather than globally. Where that is impractical, the
write can hand back its log position and the reader can wait for it:

```sql
SELECT pg_last_wal_replay_lsn();
```

`synchronous_commit = remote_apply` buys the same guarantee everywhere, at the
price of every commit waiting for a second machine, which is how a split meant
to relieve the primary ends up slower than no split at all.
