---
title: An exemplar names one observation without adding a dimension
type: tool
tags: [prometheus, exemplar, tracing, openmetrics]
---

# Exemplars point at one recorded call

### An aggregate cannot name anything, and an exemplar can
**Date**: 2026-07-18
**Source**: https://prometheus.io/docs/prometheus/latest/feature_flags/

A p99 of two point three tells you the shape of the tail and nothing about who
was in it. That gap is what makes somebody add a dimension naming the caller,
and the storage cost of that decision is paid on every scrape from then on. An
exemplar answers the same question at a fixed cost: an OpenMetrics annotation
hanging off a bucket, carrying the trace of one observation that landed there.

```
http_duration_seconds_bucket{le="2.5"} 4711 # {trace="8a3f0c"} 2.31 1721300000
```

Prometheus keeps them in a circular buffer sized once at startup, behind
`--enable-feature=exemplar-storage`, so the store cannot grow with traffic and
the oldest are overwritten. Grafana draws them as dots along the graph and links
each one through to the trace it came from, which is the workflow this exists
for: see the tail, click the tail, read the slow one.

Two limits decide whether you can lean on them. An exemplar is attached only
when the observation happened inside a sampled trace, so a collector that drops
that trace afterwards leaves a dot pointing at nothing. And they do not survive
aggregation faithfully: a sum over buckets keeps a few of the dots it saw rather
than all of them, which makes an exemplar an example and never a count. Use it
to find a case to read, and use the aggregate to decide how common the case is.
