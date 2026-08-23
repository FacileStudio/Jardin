---
title: A test that sleeps is a test that will flake
type: convention
tags: [testing, flaky, timing, ci]
---

# Waiting for something to become true

### Sleep encodes a guess about hardware you do not control
**Date**: 2026-08-01
**Source**: team decision, taken after the third flaky case bisected to a timer

`sleep(100ms)` is two mistakes at once. It is too long whenever the thing
already happened, which over a few hundred cases is a suite nobody runs
locally, and too short on a busy runner with sixteen jobs on one box, which is
a red pipeline that goes green on a rerun and teaches everybody to press rerun.

Replace it with a bounded wait: a condition, a poll interval, and a deadline.

```go
stop := time.Now().Add(5 * time.Second)
for time.Now().Before(stop) {
    if ready() {
        return
    }
    time.Sleep(10 * time.Millisecond)
}
t.Fatal("never ready, last seen: " + state())
```

Two details separate a helper that helps from one that hides. The message on
giving up has to carry the last state observed, or a timeout tells you only
that something did not happen and never what did. And the deadline belongs to
the whole wait rather than each attempt, so a chain of them cannot add up to a
suite that hangs instead of failing.
