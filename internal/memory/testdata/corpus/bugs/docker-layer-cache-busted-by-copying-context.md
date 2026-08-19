---
title: COPY . invalidates every cached layer after it
type: bug
tags: [docker, cache, build, layer]
---

# Layer cache busted by copying the whole context

### The cache key is the content of what you copy, so copying everything keys on everything
**Date**: 2026-02-22
**Source**: direct observation, every build reinstalling dependencies

A Dockerfile that copies the whole build context before installing dependencies
recomputes the cache key from every file in the repository, so editing a README
invalidates the dependency install and every layer beneath it. The build is
correct and slow, which is the worst combination for something that runs on
every push.

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
```

Copy exactly the files a step needs, in increasing order of how often they
change. A `.dockerignore` matters as much as the ordering: without one, the
context includes `.git` and any local build output, so the cache key changes
when nothing that affects the image did.
