---
title: Module-level state on the server is shared by every visitor
type: bug
tags: [svelte, state, ssr, security]
---

# Module state leaks between users

### A variable at module scope lives as long as the server process
**Date**: 2026-06-11
**Source**: direct observation, one user seeing another's dashboard

A store or a plain `$state` declared at the top level of a module is created
once per process, not once per request. In the browser that is one tab and looks
correct. On the server it is every request the process handles, so whatever the
last visitor wrote is what the next visitor reads — names, totals, occasionally
a session.

State belonging to a request must be created inside `load` and passed down, or
held in a context created per component tree. The reason this survives review so
often is that it behaves perfectly in development with one user clicking around,
and only misbehaves once two people are served by the same process at the same
time.
