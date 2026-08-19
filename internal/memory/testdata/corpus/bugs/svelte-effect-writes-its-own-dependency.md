---
title: An effect that writes state it reads loops forever
type: bug
tags: [svelte, runes, effect, reactivity]
---

# Effect loops on its own dependency

### Reading and writing the same state inside $effect never settles
**Date**: 2026-04-19
**Source**: direct observation, browser tab pegged at one core

An effect tracks every piece of reactive state it reads. Writing to something it
also read schedules the effect again, and the loop only stops when the value
stops changing — which for anything derived from a timestamp, an array copy or
an object literal is never, because each run produces a fresh reference.

```js
$effect(() => {
    total = items.reduce((n, i) => n + i.price, 0);
});
```

This particular shape is not an effect at all. Anything computed purely from
other state belongs in `$derived`, which memoises and cannot loop. Reserve
`$effect` for genuine side effects that reach outside the reactive graph:
logging, focus, a network call, a subscription that needs tearing down.
