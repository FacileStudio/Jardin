---
title: A goroutine that ignores context outlives its request
type: bug
tags: [go, goroutine, context, leak]
---

# Goroutines leak past a cancelled context

### Cancelling a context does not stop anything by itself
**Date**: 2026-02-05
**Source**: direct observation, memory climbing under load

A cancelled context closes a channel. It does not interrupt a running function,
kill a goroutine, or abort a blocking system call. Code that takes `ctx` as its
first argument and then never reads `ctx.Done()` is completely unaffected by
cancellation, and every abandoned request leaves a goroutine holding its stack
and whatever it captured.

The leak is easy to see once you look for it:

```go
go func() {
    select {
    case out <- work():
    case <-ctx.Done():
    }
}()
```

Without the second case, a caller that has walked away leaves the send blocked
forever on an unbuffered channel. `runtime.NumGoroutine` climbing while request
rate is flat is the symptom; a goroutine profile names the line.
