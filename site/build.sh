#!/usr/bin/env bash
# Full LOCAL build of the marketing site, including the download page: cross-build
# the per-arch binaries, stage them where build.mjs hashes them, then render ./dist.
# (The CI docker-site job stages the same binaries from the build artifacts — see
# .gitlab-ci.yml — so local and CI produce the same download page.)
#
#   ./site/build.sh            → bin/* + site/src/assets/downloads/* + site/dist
#
# Plain `node site/build.mjs` still works without binaries — the download page just
# shows "not staged" for the native builds (the page itself always renders).
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/.." && pwd)"

"$repo/scripts/build.sh" --ui                       # web/dist + bin/ensemble-linux-*
mkdir -p "$here/src/assets/downloads"
cp "$repo"/bin/ensemble-linux-* "$here/src/assets/downloads/"

cd "$here"
ENSEMBLE_VERSION="${ENSEMBLE_VERSION:-$(git -C "$repo" describe --tags --always 2>/dev/null || echo dev)}" \
  node build.mjs
