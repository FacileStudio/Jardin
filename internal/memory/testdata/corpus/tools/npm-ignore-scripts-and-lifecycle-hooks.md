---
title: A package runs code at install time, before anything imports it
type: tool
tags: [npm, postinstall, supply-chain, scripts]
---

# Lifecycle hooks and --ignore-scripts

### preinstall and postinstall run for every package in the tree
**Date**: 2026-01-22
**Source**: https://docs.npmjs.com/cli/v10/using-npm/scripts

The hooks execute as the invoking user, with the network and the filesystem
that user has, during `npm install` and `npm ci`. They run for transitive
packages too, which means the review you did of your direct dependencies covers
a small fraction of the code that will execute. Nothing has to be imported for
this to happen, and a container build does it as root by default.

```sh
npm ci --ignore-scripts
```

The same switch belongs in `.npmrc` as `ignore-scripts=true` so it applies to
every invocation rather than the one somebody remembered. What breaks is the
minority of packages that genuinely need a hook: native addons compiled by
node-gyp, and tools that fetch a platform binary after install. Run those
explicitly with `npm rebuild <package>` once you have decided to trust them.
That turns an open door into a short list you can name.
