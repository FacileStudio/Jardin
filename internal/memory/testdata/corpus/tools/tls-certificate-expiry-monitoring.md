---
title: Monitor certificate expiry from outside, on the real endpoint
type: tool
tags: [tls, certificate, expiry, monitoring]
---

# Certificate expiry monitoring

### Renewal succeeding is not the same as the server serving the new certificate
**Date**: 2026-06-20
**Source**: direct observation, expired certificate on a renewed host

The renewal cron ran, the file on disk was fresh, and the site served an expired
certificate for a day because nothing reloaded the server after the file
changed. Monitoring the certificate file, or the renewal exit code, misses this
entirely: the only fact that matters is what the endpoint presents.

```sh
echo | openssl s_client -connect example.com:443 2>/dev/null \
  | openssl x509 -noout -enddate
```

Alert on days remaining, from an external check, at a threshold longer than the
renewal interval so a single failed renewal is visible before it is urgent. Every
hostname needs its own check: a wildcard covers many names, but a redirect host
or an internal endpoint often carries a different certificate nobody remembers.
