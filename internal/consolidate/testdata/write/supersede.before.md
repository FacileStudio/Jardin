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
**Date**: 2026-08-01
**Source**: direct observation, local.Kit.Register in porte v0.2.2
Registration with an address that already has an account attaches the caller's
password to that account, so every SSO-only account belongs to whoever registers
its address first.

### Logout requires a live session to clear the cookie
**Date**: 2026-08-02
**Source**: direct observation, POST /auth/logout handler
Behind RequireAuth a stale cookie gets a 401 and is never cleared.
