---
title: PKCE is mandatory for any client that cannot keep a secret
type: convention
tags: [oauth, pkce, auth, token]
---

# PKCE for public clients

### An authorization code alone is bearer-grade once it leaves the browser
**Date**: 2026-02-11
**Source**: OAuth 2.1 draft, RFC 7636

A single-page app or a mobile app cannot hold a client secret: shipping one puts
it in every copy of the binary. Without a secret, an authorization code
intercepted from a redirect — a malicious app registered on the same custom
scheme, a logged referrer — can be exchanged by the attacker.

PKCE binds the code to the requester. The client sends the SHA-256 of a
one-time random verifier when it asks for the code, then presents the verifier
itself at exchange, and the server rejects any mismatch.

```
code_challenge_method=S256
```

`plain` exists for clients that cannot hash and is worth nothing against an
attacker who saw the challenge. OAuth 2.1 requires PKCE for every client
including confidential ones, on the grounds that a secret is not a good
substitute for binding the code to whoever requested it.
