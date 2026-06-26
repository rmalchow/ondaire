#!/usr/bin/env bash
# Build the ensemble playback-node firmware for a board, producing a single
# merged flash image (build-<board>/ensemble-fw-<board>.bin) ready for the web
# flasher / esptool.
#
#   ./build.sh esp32s3            # build for the S3 devkit
#   ./build.sh esp32s3 flash      # build + flash an attached board over USB
#   ./build.sh esp32s3 monitor    # build, flash, open the serial monitor
#
# Uses a local ESP-IDF if IDF_PATH (or ~/esp/esp-idf) is present; otherwise
# falls back to the espressif/idf Docker image — so it runs with no toolchain
# installed (this is exactly what CI uses).
#
# Each board gets its OWN sdkconfig inside its build dir (-DSDKCONFIG=...), so
# building multiple boards from one checkout never clobbers a shared config.
set -euo pipefail

BOARD="${1:-esp32s3}"
EXTRA="${2:-}"
IDF_VERSION="${IDF_VERSION:-release-v5.4}"   # Docker fallback image tag
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# BOARD names a board profile (boards/ + sdkconfig.defaults.<board>); TARGET is
# the IDF silicon. They differ when several boards share one chip — e.g. the
# Super Mini and the DevKitC are both esp32s3 silicon but want distinct configs.
case "$BOARD" in
  esp32s3|esp32|esp32c3|esp32c6) TARGET="$BOARD" ;;
  esp32s3-supermini)             TARGET="esp32s3" ;;
  esp32s3-zero)                  TARGET="esp32s3" ;;
  *) echo "unknown board '$BOARD' (esp32s3|esp32s3-supermini|esp32s3-zero|esp32|esp32c3|esp32c6)"; exit 2 ;;
esac

BUILD_DIR="build-$BOARD"
IDFARGS="-B $BUILD_DIR -DBOARD=$BOARD -DSDKCONFIG=$BUILD_DIR/sdkconfig"

do_build() {
  idf.py $IDFARGS set-target "$TARGET"
  idf.py $IDFARGS build
  # Single merged image at offset 0 for the web flasher (one part per chipFamily).
  idf.py $IDFARGS merge-bin -o "ensemble-fw-$BOARD.bin"
  echo "merged image: $HERE/$BUILD_DIR/ensemble-fw-$BOARD.bin"
}

# Resolve a local IDF, else use Docker.
if command -v idf.py >/dev/null 2>&1; then
  HAVE_IDF=1
elif [ -f "${IDF_PATH:-$HOME/esp/esp-idf}/export.sh" ]; then
  # shellcheck disable=SC1091
  source "${IDF_PATH:-$HOME/esp/esp-idf}/export.sh"
  HAVE_IDF=1
else
  HAVE_IDF=0
fi

cd "$HERE"

if [ "$HAVE_IDF" = "1" ]; then
  do_build
  case "$EXTRA" in
    flash)   idf.py $IDFARGS flash ;;
    monitor) idf.py $IDFARGS flash monitor ;;
  esac
else
  echo "No local ESP-IDF found — building in Docker (espressif/idf:$IDF_VERSION)…"
  docker run --rm -v "$HERE:/project" -w /project -e HOME=/tmp "espressif/idf:$IDF_VERSION" \
    bash -c "idf.py $IDFARGS set-target '$TARGET' && idf.py $IDFARGS build && idf.py $IDFARGS merge-bin -o 'ensemble-fw-$BOARD.bin'"
  echo "merged image: $HERE/$BUILD_DIR/ensemble-fw-$BOARD.bin"
fi
