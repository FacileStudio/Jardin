---
title: A log line per failed request turns a small outage into a large one
type: convention
tags: [logging, sampling, throughput, structured]
---

# Logging is on the request path

### The service fell over from writing about falling over
**Date**: 2026-04-03
**Source**: direct observation, an upstream timeout that became a self-inflicted outage

A dependency began timing out, every request took the error branch, and the
error branch logged a formatted line with a stack trace. Serialising and writing
those lines cost more than the work the service was meant to be doing, and when
the collector fell behind, the pipe buffer filled and writes to stdout blocked
the handlers themselves. The dependency recovered in two minutes. The service
took twenty.

```go
logger := zap.New(zapcore.NewSamplerWithOptions(core, time.Second, 10, 100))
```

Keep the first few occurrences of a repeated line each second and one in every
hundred after that, keyed on the message rather than the formatted string. The
varying parts belong in fields, not interpolated into the message, so the key
stays stable and a search can group by it. Reserve error for something a human
must act on, and let a per-request outcome be a metric instead.
