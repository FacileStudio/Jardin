---
title: A trace ends at the publisher and the consumer starts a new one
type: bug
tags: [opentelemetry, tracing, context, queue]
related: [[baggage-travels-with-the-trace-context]]
---

# Propagation across an async hop

### Nothing carries the parent unless you put it in the message
**Date**: 2026-06-11
**Source**: direct observation, two disjoint traces for one logical operation

The producer traced its publish, the consumer traced its handler, and the two
never appeared together. Context travels in-process through the context value
and between processes only inside carrier headers, which an HTTP client
instrumentation injects for you and a broker client does not. A message with no
`traceparent` in its metadata gives the consumer nothing to be a child of, so
the SDK does the reasonable thing and begins a fresh root.

```go
propagator.Inject(ctx, propagation.MapCarrier(msg.Headers))
```

Extract on the consuming side into the context you hand the handler. The other
half of the same bug is a goroutine started with `context.Background()` because
the request context was about to be cancelled: the work runs, and its spans land
in no trace at all. For a batch consumer, use span links rather than forcing many
parents onto one span.
