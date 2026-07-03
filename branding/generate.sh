#!/usr/bin/env bash
# Regenerate every ensemble logo asset from the Fraunces 'e', then distribute the
# favicon set into the marketing site and the brand icons into the HA integration.
# Requires: python3 + fonttools, inkscape, imagemagick (magick).
set -euo pipefail
cd "$(dirname "$0")"
REPO="$(cd .. && pwd)"

png() { inkscape "$1" --export-type=png --export-filename="$2" -w "$3" -h "$3" >/dev/null 2>&1; }
png_h() { inkscape "$1" --export-type=png --export-filename="$2" -h "$3" >/dev/null 2>&1; }

echo "→ master SVGs"
python3 extract_e.py
python3 wordmark.py

echo "→ branding/png (128–1024 marks, wordmark @160/320/640)"
mkdir -p png
for v in icon-mint-black icon-mint-white e-outline; do
  for s in 128 256 512 1024; do png "ensemble-$v.svg" "png/ensemble-$v-$s.png" "$s"; done
done
for v in wordmark wordmark-inverse; do
  for h in 160 320 640; do png_h "ensemble-$v.svg" "png/ensemble-$v-${h}h.png" "$h"; done
done

echo "→ marketing site favicon set → site/src/assets"
SITE="$REPO/site/src/assets"
cp ensemble-icon-mint-black.svg "$SITE/favicon.svg"
# multi-resolution .ico (16/32/48) rendered crisply from the SVG
tmp="$(mktemp -d)"
for s in 16 32 48; do png ensemble-icon-mint-black.svg "$tmp/f$s.png" "$s"; done
magick "$tmp/f16.png" "$tmp/f32.png" "$tmp/f48.png" "$SITE/favicon.ico"
# opaque full-bleed square for apple-touch + PWA (OS applies its own mask/rounding)
png ensemble-icon-mint-black-square.svg "$SITE/apple-touch-icon.png" 180
png ensemble-icon-mint-black-square.svg "$SITE/icon-192.png" 192
png ensemble-icon-mint-black-square.svg "$SITE/icon-512.png" 512
rm -rf "$tmp"

echo "→ Home Assistant brand icons → integrations/homeassistant/brands"
HA="$REPO/integrations/homeassistant/brands/custom_integrations/ensemble"
mkdir -p "$HA"
png ensemble-icon-mint-black.svg "$HA/icon.png" 256
png ensemble-icon-mint-black.svg "$HA/icon@2x.png" 512
# horizontal wordmark → logo (shortest side 128–256 / @2x 256–512, landscape).
# dark_logo* is served by HA in dark mode; the ink-text logo is for light mode.
png_h ensemble-wordmark.svg "$HA/logo.png" 160
png_h ensemble-wordmark.svg "$HA/logo@2x.png" 320
png_h ensemble-wordmark-inverse.svg "$HA/dark_logo.png" 160
png_h ensemble-wordmark-inverse.svg "$HA/dark_logo@2x.png" 320
magick mogrify -strip "$HA"/*.png  # brands CI wants lean PNGs

echo "✓ done"
