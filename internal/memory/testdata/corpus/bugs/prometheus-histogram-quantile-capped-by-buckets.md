---
title: A histogram quantile can never exceed its largest finite bucket
type: bug
tags: [prometheus, histogram, quantile, latency]
---

# Histogram buckets bound the answer

### p99 sat at exactly 10 seconds for a week because that was the last bucket
**Date**: 2026-02-09
**Source**: https://prometheus.io/docs/practices/histograms/

The latency panel reported a p99 of exactly 10s, unchanged across deploys and
traffic swings, while users were waiting close to a minute. `histogram_quantile`
interpolates inside whichever bucket the quantile falls into, and when it falls
into the `+Inf` bucket there is nothing above to interpolate towards, so it
returns the largest finite `le` bound. The default client buckets stop at 10, so
every slow request in the tail was reported as ten seconds and no worse.

```promql
sum by (le) (rate(http_request_duration_seconds_bucket[5m]))
```

Run that and look at the share sitting in `+Inf` compared with the bucket below
it. If the two are close, every quantile above that point is a floor rather than
a measurement. Buckets have to be chosen to cover the range you actually want to
see, including the part you consider unacceptable, because a value outside the
range is indistinguishable from one at its edge.
