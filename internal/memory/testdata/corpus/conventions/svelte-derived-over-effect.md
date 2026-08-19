---
title: Reach for $derived before $effect
type: convention
tags: [svelte, runes, derived, state]
---

# Prefer derived to effect

### If it computes a value, it is not a side effect
**Date**: 2026-04-21
**Source**: team decision, recorded after the third such review comment

The rule that survives review: `$derived` for anything you can compute from
existing state, `$effect` only for work whose whole point is to happen outside
the component's data. Derived values are lazy, memoised, and cannot desynchronise
from their inputs because they have no independent existence to desynchronise.

An effect that assigns to a `$state` variable is nearly always a derived value
written the long way. It costs an extra render pass, it can be observed in the
intermediate state where the source has changed and the copy has not, and it
opens the door to a loop. The exception worth allowing is a value that must
survive its input changing — a snapshot, an undo buffer — where independence is
the actual requirement rather than an accident.
