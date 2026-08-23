---
title: A target that disappears takes its alert with it
type: bug
tags: [prometheus, alerting, staleness, discovery]
---

# A missing series is not a false condition

### up == 0 cannot fire for a target nobody is scraping
**Date**: 2026-05-15
**Source**: direct observation, a job removed by a bad relabel rule

A relabelling change dropped one job from service discovery. Nothing alerted,
because the rule was `up == 0`, and once the target is gone Prometheus writes a
stale marker for its series and stops returning it. The expression then matches
no samples at all, which evaluates as no alert rather than as a problem. A
failing target is loud; a target that stopped existing is silent, and the second
case is the one a misconfigured deploy actually produces.

```promql
absent(up{job="api"}) or up{job="api"} == 0
```

`absent` fires when the selector returns nothing, which covers the deleted job,
the typo in a label, and the scrape config that never loaded. Pair it with a
count against the number of instances you expect, because a job that keeps one
target out of twelve still satisfies both halves of that expression.
