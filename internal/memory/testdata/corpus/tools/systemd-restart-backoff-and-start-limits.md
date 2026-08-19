---
title: A crash-looping unit stops being restarted, quietly
type: tool
tags: [systemd, restart, service, unit]
---

# Restart backoff and start limits

### StartLimitBurst gives up permanently, and nothing pages you
**Date**: 2026-04-14
**Source**: direct observation, service down for six hours

`Restart=always` restarts a failing service until it trips the start rate limit
— five starts in ten seconds by default — after which systemd marks the unit
failed and stops trying. A service crashing on a transient dependency failure at
boot exhausts that budget in seconds and then stays down, long after the
dependency recovered.

```ini
[Service]
Restart=always
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=10
```

`RestartSec` spaces attempts so the burst is not consumed instantly. Alerting on
the unit's active state rather than on the process is what catches the
give-up: the process is simply absent, and nothing logs anything further once
systemd has stopped trying.
