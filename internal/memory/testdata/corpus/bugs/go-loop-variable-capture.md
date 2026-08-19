---
title: Loop variable capture before Go 1.22
type: bug
tags: [go, loop, closure, concurrency]
---

# Loop variable capture

### Every closure saw the last value, not its own
**Date**: 2026-01-14
**Source**: direct observation on a Go 1.21 service

Before Go 1.22 the loop variable was declared once for the whole loop and
reused on each iteration, so a closure captured the variable rather than its
value. Starting a goroutine per item and printing the item gave the final
element several times, in whatever order the scheduler chose.

```go
for _, item := range items {
    item := item
    go process(item)
}
```

That shadowing line was the standard workaround and is unnecessary from Go 1.22
onward, where the variable is per-iteration. The trap now is mixed-version
codebases: a module declaring an older `go` directive keeps the old semantics
even on a new toolchain, because the language version is taken from go.mod, not
from the compiler that happens to be installed.
