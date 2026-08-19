---
title: Deleting rows does not shrink a SQLite file
type: bug
tags: [sqlite, vacuum, disk, database]
---

# The file stays large after a delete

### Freed pages are reused, not returned to the filesystem
**Date**: 2026-06-25
**Source**: direct observation, disk alert after a large cleanup

Deleting several million rows marked their pages free inside the database file
and returned nothing to the operating system. The file stays exactly as large as
its high-water mark until it is rebuilt, which is what `VACUUM` does:

```sql
VACUUM;
```

It rewrites the whole database into a new file, so it needs free space roughly
equal to the current size and takes an exclusive lock for the duration. On a WAL
database, checkpoint first or the write-ahead log holds pages the vacuum cannot
reclaim. `auto_vacuum = INCREMENTAL` avoids the big rebuild but must be set
before the tables are created, which in practice means at schema creation or
never.
