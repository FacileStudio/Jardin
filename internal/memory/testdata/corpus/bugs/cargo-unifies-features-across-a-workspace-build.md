---
title: A feature enabled by one crate reaches every crate in the build
type: bug
tags: [rust, cargo, features, workspace]
---

# Cargo computes the union of features per build

### Features are additive, and the set is decided once for the whole graph
**Date**: 2026-07-02
**Source**: https://doc.rust-lang.org/cargo/reference/features.html

A dependency is compiled once per build with the union of every feature anyone
asked for. If one member of the workspace pulls a library with default features
and another sets `default-features = false` for a no-std target, both get the
default set. The symptom is a crate that compiles in its own directory and
fails, or silently gains an allocator, when built from the workspace root.

```toml
[workspace]
resolver = "2"
```

The v2 resolver stopped unifying features across normal, build and dev
dependencies, so a feature a test needs no longer leaks into the shipped
binary. It is implied by the edition in a package manifest but not in a virtual
workspace manifest, which has no edition to read, so a workspace root that
never spelled it out is still on v1 and nothing warns loudly about it.
`cargo tree -e features` shows which dependent asked for what.
