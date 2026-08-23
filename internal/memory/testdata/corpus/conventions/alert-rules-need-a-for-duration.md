---
title: An alert with no for duration pages on a single bad evaluation
type: convention
tags: [alerting, prometheus, flapping, oncall]
---

# Hold the condition before you page

### One noisy scrape is not an incident
**Date**: 2026-05-28
**Source**: team decision after a night of resolve-and-refire pages

A latency rule with no hold time fired whenever a single evaluation crossed the
threshold and resolved on the next one. On-call collected eleven pages between
midnight and six, none of which described a state that still existed by the time
anyone opened a terminal. Adding a hold makes the rule describe a condition that
persists rather than a sample that was unlucky.

```yaml
- alert: HighLatency
  expr: job:latency:p99 > 1
  for: 10m
  keep_firing_for: 5m
```

`for` keeps the rule pending until the expression has been true at every
evaluation across the window, and a single missing evaluation resets it, so pick
a value comfortably longer than the noise and shorter than the point at which a
user gives up. `keep_firing_for` handles the other edge, where a condition
hovering on the threshold resolves and refires as two separate pages.
