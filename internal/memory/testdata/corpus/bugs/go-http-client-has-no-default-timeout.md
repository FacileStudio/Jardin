---
title: The default Go HTTP client waits forever
type: bug
tags: [go, http, timeout, client]
---

# http.DefaultClient has no timeout

### A hung upstream holds your goroutine until the process restarts
**Date**: 2026-03-08
**Source**: direct observation, worker pool drained overnight

`http.Get` and `http.DefaultClient` have no timeout of any kind. A server that
accepts a connection and then never responds leaves the call blocked
indefinitely, and every worker that reaches that upstream is consumed one by one
until nothing is left to serve real traffic.

```go
client := &http.Client{Timeout: 10 * time.Second}
```

`Timeout` covers the whole exchange including reading the body, which is what
you usually want. A per-request `context.WithTimeout` composes with it and is
the right tool when different endpoints deserve different budgets. Neither
helps if the body is never closed: an unread, unclosed body keeps the connection
out of the pool, and the pool then blocks new requests just as thoroughly as a
missing timeout does.
