#!/usr/bin/env bash
# Regenerate every ondaire raster asset from the hand-authored mark SVGs in this
# directory, then distribute the favicon set into the marketing site and the
# brand icons into the HA integration.
# Requires: inkscape, imagemagick (magick).
#
# The mark itself (badge rect + "o" glyph + "ndaire" wordmark paths) is NOT
# generated from a font — the geometry lives directly in the SVGs here
# (ondaire-mark-mint-{dark,white}.svg, ondaire-lockup-{light,dark}.svg, plus
# the full-bleed ondaire-mark-mint-square.svg for touch icons). Edit those
# SVGs directly if the mark ever changes; there is no extraction step to rerun.
set -euo pipefail
cd "$(dirname "$0")"
REPO="$(cd .. && pwd)"

png() { inkscape "$1" --export-type=png --export-filename="$2" -w "$3" -h "$3" >/dev/null 2>&1; }
png_h() { inkscape "$1" --export-type=png --export-filename="$2" -h "$3" >/dev/null 2>&1; }

echo "→ branding/png (128–1024 marks, lockups @160/320/640 tall)"
mkdir -p png
for v in mark-mint-dark mark-mint-white; do
  for s in 128 256 512 1024; do png "ondaire-$v.svg" "png/ondaire-$v-$s.png" "$s"; done
done
for v in lockup-light lockup-dark; do
  for h in 160 320 640; do png_h "ondaire-$v.svg" "png/ondaire-$v-${h}h.png" "$h"; done
done

echo "→ marketing site favicon set → site/src/assets"
SITE="$REPO/site/src/assets"
cp ondaire-mark-mint-dark.svg "$SITE/favicon.svg"
# multi-resolution .ico (16/32/48) rendered crisply from the SVG
tmp="$(mktemp -d)"
for s in 16 32 48; do png ondaire-mark-mint-dark.svg "$tmp/f$s.png" "$s"; done
magick "$tmp/f16.png" "$tmp/f32.png" "$tmp/f48.png" "$SITE/favicon.ico"
# opaque full-bleed square for apple-touch + PWA (OS applies its own mask/rounding)
png ondaire-mark-mint-square.svg "$SITE/apple-touch-icon.png" 180
png ondaire-mark-mint-square.svg "$SITE/icon-192.png" 192
png ondaire-mark-mint-square.svg "$SITE/icon-512.png" 512
rm -rf "$tmp"

echo "→ Home Assistant brand icons → integrations/homeassistant/brands"
HA="$REPO/integrations/homeassistant/brands/custom_integrations/ondaire"
mkdir -p "$HA"
png ondaire-mark-mint-dark.svg "$HA/icon.png" 256
png ondaire-mark-mint-dark.svg "$HA/icon@2x.png" 512
# horizontal lockup → logo (shortest side 128–256 / @2x 256–512, landscape).
# dark_logo* is served by HA in dark mode; the ink-text logo is for light mode.
png_h ondaire-lockup-light.svg "$HA/logo.png" 160
png_h ondaire-lockup-light.svg "$HA/logo@2x.png" 320
png_h ondaire-lockup-dark.svg "$HA/dark_logo.png" 160
png_h ondaire-lockup-dark.svg "$HA/dark_logo@2x.png" 320
magick mogrify -strip "$HA"/*.png  # brands CI wants lean PNGs

echo "✓ done"
