---
title: Daemon install gating
type: projects
sources:
  - direct observation
related: []
confidence: high
created: 2026-08-10
updated: 2026-08-10
---

### The daemon regenerates agent configs every tick unless stamped
**Date**: 2026-08-10
**Source**: direct observation, internal/daemon/daemon.go
The daemon ticks every 60s and regenerating agent configs is write-heavy, so the
install step is gated by a .last-install stamp file checked every five minutes.

### The fix was to run sessions scan before each sync in the daemon so
**Date**: 2026-08-24
**Source**: consolidation, pi@2026-08-24T10:00:00Z
The fix was to run sessions scan before each sync in the daemon so the shards are sealed before the files ride the sync.
