---
title: The sampling decision belongs to the root and travels with the trace
type: convention
tags: [opentelemetry, sampling, tracing, propagation]
---

# Sample once, then obey

### Independent sampling at each service leaves traces with holes
**Date**: 2026-07-02
**Source**: https://opentelemetry.io/docs/concepts/sampling/

Four services each configured a ten percent probability sampler, which meant a
complete four-hop trace survived one time in ten thousand. What was collected
looked worse than nothing: spans present at the edge, missing in the middle,
present again at the database, with the gaps invisible rather than marked. The
sampled flag is already carried in `traceparent`, so a downstream service does
not need an opinion, it needs to respect the one it was given.

```
ParentBased(root: TraceIDRatioBased(0.1))
```

Every service after the root uses the parent decision and only the root rolls
the dice. When the interesting traces are the failures, head sampling cannot
help, because the decision is made before the error exists. That case needs tail
sampling in a collector that buffers the whole trace and keeps it on latency or
status, at the cost of holding spans in memory until the trace is complete.
