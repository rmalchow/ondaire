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

# tar.gz each binary (renamed to `ensemble` inside, so it extracts to ./ensemble).
dl="$here/src/assets/downloads"
mkdir -p "$dl"
rm -f "$dl"/ensemble-linux-*.tar.gz
for bin in "$repo"/bin/ensemble-linux-*; do
  name="$(basename "$bin")"                          # ensemble-linux-arm64
  tmp="$(mktemp -d)"
  cp "$bin" "$tmp/ensemble"
  tar -czf "$dl/$name.tar.gz" -C "$tmp" ensemble
  rm -rf "$tmp"
done

# Stage any ESP32 firmware images already built locally (esp32/build-<board>/) so
# the flasher in ./dist actually works — mirrors the CI docker-site job, which
# stages fw/ensemble-fw-*.bin the same way. Build an image first with, e.g.,
# `esp32/build.sh esp32s3-supermini`; if none exist the flasher just shows
# "not built" for that board. (build.mjs also falls back to esp32/build-<id>/.)
fw="$here/src/assets/firmware"
mkdir -p "$fw"
rm -f "$fw"/ensemble-fw-*.bin
shopt -s nullglob
for bin in "$repo"/esp32/build-*/ensemble-fw-*.bin; do
  cp "$bin" "$fw/$(basename "$bin")"
done
shopt -u nullglob

cd "$here"
ENSEMBLE_VERSION="${ENSEMBLE_VERSION:-$(git -C "$repo" describe --tags --always 2>/dev/null || echo dev)}" \
  node build.mjs
