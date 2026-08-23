---
title: npm install rewrites the lockfile, npm ci obeys it
type: convention
tags: [npm, node, lockfile, ci]
---

# Install from the lockfile on a build machine

### A caret range resolves to whatever was newest that morning
**Date**: 2026-02-11
**Source**: https://docs.npmjs.com/cli/v10/commands/npm-ci

`npm install` treats `package.json` as the source of truth and the lockfile as a
suggestion. If a newer version still satisfies the declared range it installs
that one and rewrites `package-lock.json` to match. The build machine therefore
resolves the tree again, and a developer who never touched dependencies finds
the lockfile modified in an unrelated pull request.

```sh
npm ci --omit=dev
```

`npm ci` deletes `node_modules`, installs exactly the tree the lockfile
describes, and refuses to update it. When `package.json` and the lockfile
disagree it fails instead of picking a winner, which is the behaviour worth
having in a pipeline: the disagreement is a real one and belongs in front of a
person.

### A lockfile from a different npm major reshuffles the whole file
**Date**: 2026-02-11
**Source**: direct observation, `lockfileVersion` changing from 2 to 3

An install run by an older or newer npm rewrites the format, so the diff is
thousands of lines and hides whatever changed. Pin the toolchain under
`engines` and install it in the pipeline the same way as the runtime.
