---
title: BuildKit cache mounts survive between builds
type: tool
tags: [docker, build, cache, ci]
---

# BuildKit cache mounts

### A cache mount is not a layer and is not invalidated by a changed file
**Date**: 2026-02-18
**Source**: https://docs.docker.com/build/cache/

Copying a lockfile before the source is the classic way to keep dependency
installation out of a rebuild, but it still redownloads everything whenever the
lockfile itself changes. A cache mount keeps the package directory on the
builder across builds regardless of layer invalidation:

```dockerfile
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /out/app .
```

Two things surprise people. The mount is not present in the resulting image, so
nothing you write there is shipped. And the cache is per builder, so a fresh CI
runner starts cold no matter what the Dockerfile says — the win is on repeat
builds on a warm machine, not on the first build of a pull request.
