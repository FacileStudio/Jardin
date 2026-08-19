---
title: A retry without an idempotency key is a duplicate
type: convention
tags: [http, retry, idempotency, api]
---

# Retries need idempotency keys

### A timeout tells you nothing about whether the work happened
**Date**: 2026-03-17
**Source**: team decision after a double-charge incident

A client that times out waiting for a response cannot distinguish a request that
never arrived from one that succeeded and whose response was lost. Retrying is
correct; retrying blindly is how a customer is charged twice. The server has to
be able to recognise the second attempt as the same attempt.

```http
POST /v1/charges
Idempotency-Key: 0f9c2b1e-4d55-4a6b-9a09-8a3b1f6f5e21
```

The key is generated once per logical operation by the caller and reused across
every retry of it, including retries after a process restart, which means it
belongs in the caller's durable state rather than in memory. The server stores
the key with the response for long enough to outlive any client retry policy and
replays that response verbatim rather than doing the work again.
