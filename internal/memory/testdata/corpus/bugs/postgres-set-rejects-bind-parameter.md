---
title: Postgres SET does not accept a bind parameter
type: bug
tags: [postgres, sql, database, migration]
---

# Postgres SET rejects a placeholder

### Configuration statements are parsed before parameters are bound
**Date**: 2026-04-02
**Source**: direct observation, `pq: syntax error at or near "$1"`

`SET search_path = $1` fails with a syntax error however the driver sends it.
`SET`, `SET ROLE` and `SET LOCAL` are utility statements, not queries: the
parameter substitution the wire protocol performs never applies to them, so the
placeholder reaches the parser literally.

The only safe form is a quoted identifier built server side:

```sql
SELECT set_config('search_path', $1, false);
```

`set_config` is an ordinary function call, so it takes a real bind parameter and
carries no injection risk. Interpolating the value into the `SET` string
yourself works and is how most people discover the problem, but it puts
untrusted text directly into a statement, which is the trade this note exists to
prevent anyone making twice.
