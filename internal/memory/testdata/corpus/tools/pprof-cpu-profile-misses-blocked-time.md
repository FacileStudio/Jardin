---
title: A CPU profile is blind to time spent waiting
type: tool
tags: [pprof, profiling, latency, go]
---

# Profiling a service that is not busy

### A flame graph is wide where the CPU was, not where the time went
**Date**: 2026-02-18
**Source**: https://go.dev/blog/pprof

Requests were taking four seconds and a thirty second CPU profile came back
nearly empty, a few percent spent in the JSON encoder and nothing else. That is
the expected result: the profiler samples running stacks at a fixed rate, so a
goroutine parked on a lock, a channel or a socket contributes no samples at all.
The flame graph was accurate and useless, because the service was not slow from
computing anything.

```go
runtime.SetBlockProfileRate(10000)
runtime.SetMutexProfileFraction(100)
```

Both are off by default and cost something when enabled, so turn them on
deliberately and read `/debug/pprof/block` and `/debug/pprof/mutex` afterwards.
For a one-off, `/debug/pprof/goroutine?debug=2` dumps every stack with how long
it has been parked, which usually names the culprit faster than any profile: a
thousand goroutines stopped on the same line is not a subtle finding.
