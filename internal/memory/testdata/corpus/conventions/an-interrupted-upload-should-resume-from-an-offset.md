---
title: A transfer that dies at ninety percent should not start over
type: convention
tags: [upload, resume, http, storage]
---

# Resumable transfers

### The client needs a way to ask how much arrived
**Date**: 2026-07-31
**Source**: team decision, after a week of failed video ingests

A single POST carrying a whole file has one failure mode and it is total. A
dropped socket at ninety percent costs everything sent so far, and on a mobile
connection that can mean the transfer never completes at all, however many times
somebody retries it. The wasted work is what makes people give up.

The shape that survives is three verbs. Create the transfer and get back a URL
that identifies it. Send bytes to that URL with a `PATCH` naming the offset they
begin at. Ask that URL with a `HEAD` how many bytes it already holds, which is
what a client does on resume to decide where to continue from.

```http
HEAD /uploads/9f2c
Upload-Offset: 41943040
```

That is the tus protocol in three lines, and the reason to copy it rather than
invent one is that browser and mobile clients for it already exist.

Two rules keep an implementation honest. The offset must be read from durable
storage rather than from a counter in memory, because the worker that accepted
the earlier bytes is not the worker answering the resume. And every transfer
needs an expiry with something that sweeps the abandoned ones: a client that
never returns leaves bytes billed like any others and listed nowhere.
