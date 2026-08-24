---
title: Porte auth kit
type: bugs
sources:
  - direct observation
related: []
confidence: high
created: 2026-08-01
updated: 2026-08-01
---

### Register attaches the password to any existing address
~~**Date**: 2026-08-01
**Source**: direct observation, local.Kit.Register in porte v0.2.2
Registration with an address that already has an account attaches the caller's
password to that account, so every SSO-only account belongs to whoever registers
its address first.~~ [SUPERSEDED by: consolidation, 2026-08-24]

Register refuses the password when the address already belongs to a passwordless account, so SSO-only accounts can no longer be claimed by registering their address.
**Date**: 2026-08-24
**Source**: consolidation, pi@2026-08-24T10:00:00Z

### Logout requires a live session to clear the cookie
**Date**: 2026-08-02
**Source**: direct observation, POST /auth/logout handler
Behind RequireAuth a stale cookie gets a 401 and is never cleared.
