---
title: One unbounded label multiplies every series on a metric
type: bug
tags: [prometheus, cardinality, memory, metrics]
related: [[exemplars-point-at-one-recorded-call]]
---

# Cardinality is a product, not a sum

### A user id in a label turned one metric into four hundred thousand series
**Date**: 2026-01-22
**Source**: direct observation, out-of-memory restarts on the scraper

Someone added a `user_id` label to an existing request counter to answer one
support question. The metric already carried method, route, status and instance,
so the new label did not add series, it multiplied them by the number of active
users. Head series climbed for two days, query latency followed, and the server
began restarting on memory pressure before anyone connected the change to it.

```promql
topk(10, count by (__name__) ({__name__=~".+"}))
```

Any label whose values come from user input, a generated identifier, a full URL
path or an error message is unbounded by construction and does not belong on a
metric. Identifiers belong on a trace or a log line, where the storage cost is
linear rather than multiplicative, and where you can look one up without paying
for all of them.
