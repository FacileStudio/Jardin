---
title: A counter restarts at zero and a raw subtraction goes negative
type: bug
tags: [prometheus, counter, restart, dashboard]
---

# Counter resets and raw deltas

### rate corrects for a reset, arithmetic does not
**Date**: 2026-04-27
**Source**: direct observation, request totals dipping below zero after a deploy

A panel computed daily volume as the value now minus the value twenty-four hours
ago. Every deploy restarted the process, the counter began again at zero, and
the subtraction produced a negative number that the stacked graph rendered as a
gap. `rate` and `increase` detect the drop, treat it as a reset, and add the
pre-reset value back, which is exactly why they exist and why no dashboard
should subtract two samples of a counter directly.

```promql
increase(http_requests_total[24h])
```

The same trap appears when application code re-registers or reinitialises a
collector at runtime: from the outside that is indistinguishable from a restart,
and any window spanning it undercounts. Counters are only meaningful through a
function that understands resets, and a value that can go down was never a
counter in the first place.
