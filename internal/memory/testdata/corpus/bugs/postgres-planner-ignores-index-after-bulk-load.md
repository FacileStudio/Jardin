---
title: The planner ignores a new index until statistics are refreshed
type: bug
tags: [postgres, index, planner, database]
---

# Planner ignores an index after a bulk load

### Statistics are not updated by the load, so the planner still thinks the table is empty
**Date**: 2026-05-11
**Source**: direct observation, sequential scan over forty million rows

A table loaded with `COPY` and then indexed still produced sequential scans. The
index existed and was valid; the planner simply had statistics from when the
table held a few hundred rows, so a sequential scan genuinely looked cheaper on
the numbers it had.

```sql
ANALYZE orders;
```

Autovacuum gets to it eventually, but eventually is measured against the
insert threshold and can be a long time on a table that is loaded once and then
read. Run `ANALYZE` as the last step of any bulk load. `EXPLAIN (ANALYZE,
BUFFERS)` comparing estimated against actual row counts is what tells you the
statistics are stale rather than the query being wrong.
