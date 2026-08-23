---
title: pip walks backwards through releases when a constraint cannot be met
type: bug
tags: [python, pip, resolver, dependencies]
---

# pip install downloads the same package over and over

### An unsatisfiable upper bound turns resolution into a long search
**Date**: 2026-05-13
**Source**: https://pip.pypa.io/en/stable/topics/dependency-resolution/

The resolver has to read a candidate's metadata to learn its requirements, and
for a source distribution that means downloading and building it. When two
requirements cannot both be satisfied, pip tries the next older release of one
of them, then the next, printing a line per attempt until it gives up or the
network does. It looks like a hang or a retry loop and it is neither: it is a
search over a graph that has no solution.

```
INFO: pip is looking at multiple versions of ... to determine which version is compatible
INFO: This is taking longer than usual. You might need to provide the dependency resolver with stricter constraints
```

The message is accurate advice. Pin the package whose range is being explored,
usually visible as the one named in every attempt, and the search space
collapses to nothing. Underneath there is almost always a single library with
an over-tight upper bound on a shared dependency, and the durable fix is that
bound, not the pin. Wheels shorten the loop because metadata comes without a
build, but they do not end it.
