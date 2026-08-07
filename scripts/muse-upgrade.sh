#!/bin/sh
# Re-pin the muse dependency at a tag and re-run the client gate.
#
#   mise run muse 0.4.0
#
# Two things this exists to encode:
#   - `bun add "github:FacileStudio/muse#v0.4.0"` does NOT work on a dependency that is
#     already present. Bun refuses to re-resolve a git dep in place when the SHA changes and
#     fails with `DependencyLoop`. Editing package.json and running a plain install does work.
#   - The lockfile then records the annotated tag *object's* SHA, not the commit's, so the
#     hash will not match `git log` in the muse repo. That is normal — reconcile with
#     `git rev-parse v0.4.0^{}`. Verify an upgrade by grepping the installed tree for the
#     symbol you expect, never by reading the version string.
set -eu

VERSION="${1:-}"
[ -n "$VERSION" ] || { echo "usage: mise run muse <version>   (e.g. 0.4.0, no leading v)" >&2; exit 1; }
echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "muse: version must be plain semver, no leading v: got '$VERSION'" >&2; exit 1; }

cd "$(dirname "$0")/../apps/client"

# Portable in-place edit: BSD and GNU sed disagree about -i, so write through a temp file.
sed "s|muse#v[0-9]*\.[0-9]*\.[0-9]*|muse#v$VERSION|" package.json > package.json.tmp
mv package.json.tmp package.json
grep -q "muse#v$VERSION" package.json || { echo "muse: the pin did not change — is the dependency declared?" >&2; exit 1; }

grep '@facile/muse' package.json
bun install

# `--bun` because `mise run` resolves an ambient /usr/local/bin/node 20.11 on this machine
# while Vite 7 needs 20.19+, and the failure it produces ("node:util does not provide an
# export named styleText") says nothing about what actually went wrong. bun is the suite's
# declared runtime, so pinning to it removes the dependency on whatever node is on PATH.
bun run --bun check
bun run --bun build

echo "muse: client is on v$VERSION and the gate is green"
