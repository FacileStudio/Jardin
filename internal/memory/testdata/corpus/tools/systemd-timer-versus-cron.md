---
title: systemd timers over cron for anything that can fail
type: tool
tags: [systemd, timer, cron, scheduling]
---

# systemd timers versus cron

### A timer gives you logs, dependencies and a catch-up run
**Date**: 2026-01-30
**Source**: https://www.freedesktop.org/software/systemd/man/systemd.timer.html

Cron mails output nowhere anybody reads, runs with a minimal environment that
differs from a login shell, and forgets any occurrence the machine slept
through. A timer unit fixes each of those:

```ini
[Timer]
OnCalendar=*-*-* 03:00:00
Persistent=true
RandomizedDelaySec=300
```

`Persistent=true` runs the job once after boot if the scheduled moment passed
while the machine was off. Output goes to the journal with the unit's name
attached, so `journalctl -u backup.timer` is a real investigation rather than a
search of a mail spool. The cost is two files instead of one line, and the
scheduling syntax being calendar-based rather than the five cron fields most
people can already read.
