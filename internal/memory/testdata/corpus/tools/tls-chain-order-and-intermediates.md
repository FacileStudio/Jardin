---
title: TLS chain order and the missing intermediate
type: tool
tags: [tls, certificate, chain, https]
---

# TLS certificate chain order

### It works in your browser because your browser already had the intermediate
**Date**: 2026-05-15
**Source**: direct observation, `x509: certificate signed by unknown authority`

A server must send its leaf certificate first and then every intermediate up to,
but not including, the root. Omit an intermediate and browsers usually still
succeed, because they cache intermediates seen on other sites or fetch them
through the authority information access extension. Go, curl and most language
runtimes do neither, so the same endpoint fails from a service and works from a
laptop.

```sh
openssl s_client -connect example.com:443 -showcerts </dev/null
```

Count the certificates in the output. Order matters as well as presence: a chain
sent leaf-last is rejected by strict clients even when every certificate is
there. The full chain file, not the certificate file, is what belongs in the
server configuration.
