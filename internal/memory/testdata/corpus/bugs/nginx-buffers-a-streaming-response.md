---
title: nginx buffers a streaming response until the handler finishes
type: bug
tags: [nginx, proxy, buffering, streaming]
---

# Streaming through a proxy

### proxy_buffering hides every partial write
**Date**: 2026-06-02
**Source**: direct observation, server-sent events arriving all at once

The handler flushed after every event and the browser received nothing until the
connection closed, at which point all of them arrived together. nginx buffers a
proxied response by default, collecting it until the buffer fills or the
upstream finishes, which is the right behaviour for a page and exactly wrong for
server-sent events or a chunked progress stream.

```nginx
location /events {
    proxy_pass http://app;
    proxy_buffering off;
    proxy_read_timeout 1h;
}
```

The read timeout matters as much as the buffering: the default of sixty seconds
closes a healthy but quiet stream. `X-Accel-Buffering: no` on the response is the
per-response equivalent when the location block is shared with ordinary traffic.
