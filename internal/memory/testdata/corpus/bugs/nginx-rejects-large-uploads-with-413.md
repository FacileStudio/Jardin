---
title: 413 on upload comes from the proxy, not the application
type: bug
tags: [nginx, proxy, upload, limits]
related: [an-interrupted-upload-should-resume-from-an-offset, nginx-buffers-a-streaming-response]
---

# 413 Request Entity Too Large

### The default body limit is one megabyte and the app never sees the request
**Date**: 2026-03-25
**Source**: direct observation, uploads failing with no application log line

Uploads over one megabyte returned 413 with nothing in the application log,
because nginx rejects the body before proxying anything upstream.
`client_max_body_size` defaults to 1m and applies per server or location block.

```nginx
location /upload {
    client_max_body_size 100m;
    proxy_request_buffering off;
    proxy_pass http://app;
}
```

Raising the limit alone still buffers the whole body to disk before the upstream
sees a byte, which for large files doubles the write and delays the first
response. Turning request buffering off streams it through. The application's
own limit has to be raised to match, or the same rejection simply moves one hop
later and looks like a different bug.
