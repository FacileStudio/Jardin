---
title: A minimal container has no trust anchors, so every outbound TLS call is refused
type: bug
tags: [docker, tls, certificates, alpine]
---

# The container carries no certificate store

### A binary that talks to the internet needs files it did not bring with it
**Date**: 2026-08-04
**Source**: direct observation, every outbound HTTPS call refused inside a scratch container

The binary was built on a full machine, tested there, and copied into a
container holding nothing but itself. Every outbound HTTPS call then failed
while the same binary on the build machine was perfectly happy. Go, Python and
curl all read their trust anchors from the filesystem, normally
`/etc/ssl/certs/ca-certificates.crt`, and an empty container has no such path,
so there is nothing to validate a peer against and the connection is refused
before any bytes are exchanged.

```dockerfile
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
```

Copying the bundle out of the build stage is the smallest repair. `apk add
--no-cache ca-certificates` is the alpine form, and the distroless static tag
already carries one, which is why the very same binary behaves differently
there and why the difference is so easy to blame on the code.

The absence generalises, and that is the part worth remembering. A minimal
container also carries no timezone database, so a call naming `Europe/Paris`
fails when a report is rendered rather than at startup. It carries no
`/etc/passwd`, so a library asking who the current user is gets a number or a
failure. It has no `/tmp` unless something creates it, and no
`/etc/nsswitch.conf`, which on some runtimes changes how a name is resolved.
