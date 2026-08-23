---
title: An undeclared import works because the package got hoisted
type: bug
tags: [npm, node-modules, hoisting, dependencies]
---

# Node resolves what is on disk, not what you declared

### A transitive package flattened to the top level is importable
**Date**: 2026-04-21
**Source**: direct observation, `Error: Cannot find module 'ms'`

npm and yarn classic flatten the tree, so a package that only ever arrived as a
dependency of a dependency ends up in the top-level `node_modules` where the
resolver finds it. Code can import it, tests pass, review notices nothing,
and `package.json` never mentions it. The failure arrives later, in a deploy
whose only change was an unrelated upgrade that deduped or dropped the
transitive copy.

```sh
npm ls ms
```

If the output shows the package only underneath something else, the import is
borrowed rather than owned. pnpm makes this impossible by symlinking each
package's own declared dependencies and nothing more, which is why a migration
to it surfaces a pile of these at once. Fixing them is a one-line addition per
package, and the layout is deterministic given a lockfile, so the bug hides
until a version somewhere in the graph moves.
