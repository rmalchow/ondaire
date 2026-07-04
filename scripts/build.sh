#!/usr/bin/env bash
# Build ondaire for linux/amd64 + arm64 into bin/, plus a host-arch ./ondaire
# for local runs (dev2.sh / e2e.sh). Pure Go (CGO_ENABLED=0), so cross-compiling
# needs no toolchain. The committed web/dist placeholder makes go:embed compile
# without node; pass --ui to (re)build and embed the SPA.
#   ./scripts/build.sh          -> bin/ondaire-linux-{amd64,arm64} + ./ondaire
#   ./scripts/build.sh --ui     -> SPA build first, then the same
#
# 64-bit only: 32-bit ARM (armv6/armhf, Pi Zero / Pi 1) is no longer supported —
# the soft-/hard-float ELF-loader split (armel /lib/ld-linux.so.3 vs armhf
# /lib/ld-linux-armhf.so.3) was never reliable enough. Use Raspberry Pi OS 64-bit.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ "${1:-}" == "--ui" ]]; then
  ./scripts/ui.sh
fi

VER="${VERSION:-$(git describe --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X main.version=$VER"

mkdir -p bin
for spec in "amd64:amd64" "arm64:arm64"; do
  IFS=: read -r name goarch <<<"$spec"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "bin/ondaire-linux-$name" ./cmd/ondaire
  echo "built bin/ondaire-linux-$name"
done

# Host-arch convenience binary at the repo root.
case "$(uname -m)" in
  x86_64)  cp "bin/ondaire-linux-amd64" ondaire ;;
  aarch64) cp "bin/ondaire-linux-arm64" ondaire ;;
  *)       CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o ondaire ./cmd/ondaire ;;
esac
echo "built ./ondaire ($VER)"
