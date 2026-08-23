---
title: ERESOLVE is a peer dependency conflict, not a broken registry
type: bug
tags: [npm, peer-dependencies, install, node]
---

# npm aborts an install it used to warn about

### From npm 7 the peer graph is enforced rather than reported
**Date**: 2026-03-05
**Source**: direct observation, `npm ERR! ERESOLVE unable to resolve dependency tree`

npm 6 printed an unmet peer dependency as a warning and finished the install.
npm 7 and later install peers automatically and treat an unsatisfiable one as a
hard failure, so upgrading a linter or a framework by one major breaks every
plugin that still declares the old range as a peer. Nothing is wrong with the
registry and retrying does not help.

```sh
npm install --legacy-peer-deps
```

That flag restores the npm 6 behaviour for the entire tree, which makes it a
fine way to get unblocked and a poor thing to commit. The narrow fix is an
`overrides` entry that pins the disputed package for the one dependency that is
behind, or a plugin release that widens its range. `--force` is a different and
worse thing: it installs a tree npm has already decided is broken.
