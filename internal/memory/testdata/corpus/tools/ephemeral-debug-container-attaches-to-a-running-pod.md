---
title: Debug a container that holds no tools by attaching one that does
type: tool
tags: [kubernetes, debugging, container, ephemeral]
---

# Attach a toolbox to a running pod

### The container you cannot exec into is still inspectable from beside it
**Date**: 2026-07-09
**Source**: https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pods/

A container built from `scratch` holds one binary and nothing else, so `exec`
has nothing to execute and lands you at `executable file not found in $PATH`.
The service is healthy and the toolbox is what is missing, so bring one rather
than rebuild the container around a package manager it does not need.

```sh
kubectl debug -it mypod --image=busybox:1.36 --target=app
```

`--target` puts the new container in the same process namespace as `app`, which
is what makes `ps`, `/proc/1/environ` and `nsenter` see the real thing. Without
it you get the same network namespace only, still enough for `curl`, `dig` and
`ss` against the addresses the service itself resolves.

Two properties matter before an incident rather than during one. An ephemeral
container cannot be removed once added, only the whole pod can, so this is an
investigation step and never a repair. And process namespace sharing has to be
allowed by the cluster policy: where it is denied, `--target` is rejected and
the fallback is `kubectl debug --copy-to`, which starts a duplicate pod with the
entrypoint replaced and leaves the original serving traffic untouched.

For a plain container runtime the same trick is `docker run --pid=container:x`,
and on a host with neither, `nsenter -t <pid> -a` reaches it from the node.
