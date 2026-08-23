---
title: A rate window narrower than two scrapes returns nothing
type: bug
tags: [grafana, promql, rate, dashboard]
---

# Empty panels when you zoom in

### $__interval shrinks with the time range until the window holds one sample
**Date**: 2026-03-05
**Source**: https://grafana.com/docs/grafana/latest/datasources/prometheus/query-editor/

The throughput panel looked correct over a day and went blank when anyone zoomed
to the last fifteen minutes. `rate` needs at least two samples inside its window
to compute a slope, and Grafana derives `$__interval` from the panel width and
the selected range, so a narrow range on a narrow panel produced a window
shorter than the sixty second scrape interval. One sample in, no value out, for
every series at once.

```promql
rate(http_requests_total[$__rate_interval])
```

`$__rate_interval` is defined as at least four times the scrape interval, which
keeps the window wide enough whatever the zoom level. The same failure appears
with a hardcoded `[1m]` against a job scraped every minute, where it works until
one scrape is late. Treat any window under four scrape intervals as a graph that
will empty itself when someone looks closely.
