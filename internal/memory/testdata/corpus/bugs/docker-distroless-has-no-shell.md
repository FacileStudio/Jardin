---
title: A distroless image has no shell, so shell-form healthchecks never run
type: bug
tags: [docker, distroless, healthcheck, deploy]
---

# Distroless images have no shell

### HEALTHCHECK in shell form silently fails on a distroless base
**Date**: 2026-05-07
**Source**: direct observation, container reported unhealthy from first boot

`HEALTHCHECK CMD curl -f http://localhost:8080/health` is shell form: Docker
wraps it in `/bin/sh -c`. A `gcr.io/distroless/static` image contains no shell
and no curl, so the probe fails immediately and the container is marked
unhealthy while the process inside is answering requests perfectly.

The fix is to make the binary its own probe and invoke it in exec form:

```dockerfile
HEALTHCHECK CMD ["/app", "healthcheck"]
```

The same trap catches `ENTRYPOINT` written as a string, any `RUN` in a final
distroless stage, and `docker exec` debugging. If you need a shell for
troubleshooting, the `:debug` variants ship busybox; the plain tags deliberately
do not.
