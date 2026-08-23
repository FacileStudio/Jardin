---
title: Baggage carries key and value pairs to every later hop
type: tool
tags: [opentelemetry, baggage, context, propagation]
---

# Baggage rides beside the trace context

### Once a carrier exists, anything can travel in it, which is the risk
**Date**: 2026-07-27
**Source**: https://www.w3.org/TR/baggage/

`traceparent` says which operation this is part of. `baggage` is the second
header of the same family and says something about the whole operation that
later hops would otherwise have to look up: the tenant, the experiment arm, the
fact that this came from a synthetic check and must not count in the numbers.

```
baggage: tenant=acme,arm=b,synthetic=1
```

The value is set once at the edge and read anywhere below it, and it survives
every hop that runs a propagator, so a handler four services deep can label its
own work without a lookup and without a parameter threaded through eight
signatures.

Three cautions, all learned the same way. Every entry is copied onto every
outbound call for the rest of the operation, so a value nobody reads is pure
weight on the wire, and some gateways cap total header size at eight kilobytes.
Nothing is confidential: it crosses any boundary the context crosses, including
to a third party, so an account name or a token in there is a leak with a long
tail. And baggage is not attached to a recorded operation automatically, since
the two are separate concerns in the specification: a processor has to copy the
entries you want onto the record, which also means choosing them deliberately
instead of promoting the lot and inheriting whatever the edge decided to set.
