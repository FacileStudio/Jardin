---
title: Add the column, write both, backfill, then switch reads
type: convention
tags: [migration, database, backfill, deploy]
---

# Changing what a table is keyed on

### The switch is four deploys, not one
**Date**: 2026-08-12
**Source**: team decision, after a migration that held an exclusive lock for nine minutes

Replacing the value a table is looked up by cannot be a single change, because
old and new binaries run together during any rolling deploy. One deploy per step:

1. Add the new column nullable, no constraint on it, and nothing reading it.
2. Write both values on every insert and update, so new rows are complete.
3. Backfill the old rows in bounded batches, then confirm no nulls remain.
4. Add the unique constraint, switch reads to it, then stop writing the old one.

The backfill is where this goes wrong. A single statement across ten million rows
holds one transaction, blocks anything wanting the table, and leaves nothing to
resume from when it is killed at minute eight.

```sql
UPDATE people SET org_ref = lookup(org_name) WHERE org_ref IS NULL LIMIT 5000;
```

Loop that until it reports no rows, pausing between passes so replication and
autovacuum keep up. Add the constraint as `NOT VALID` and `VALIDATE` it in a
second statement, which needs a weaker lock and moves the long part out of the
exclusive window. Step four is the one people skip, and skipping it turns a
reversible change into an outage: while both values are written, every step
above can be undone by deploying the previous binary.
