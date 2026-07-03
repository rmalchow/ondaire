#!/usr/bin/env python3
"""Emit the ensemble logo SVGs from the Fraunces 'e' (weight 900).

The glyph is pulled straight from the marketing site's own subsetted Fraunces
variable font so the logo matches the site 1:1. Outlines are baked into 512x512
canvas coordinates (no <transform>) so stroke widths are in plain viewBox units.

Run via ./generate.sh (which also rasterizes and distributes the assets).
"""
import os
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

HERE = os.path.dirname(os.path.abspath(__file__))
FONT = os.path.join(HERE, "..", "site", "src", "assets", "fonts", "fraunces-wght.woff2")

MINT = "#35e3b3"
INK = "#11151a"
WHITE = "#ffffff"
SIZE = 512.0
RX = 114.0  # rounded-square corner radius (~22%, squircle-ish)

f = TTFont(FONT)
instantiateVariableFont(f, {"wght": 900}, inplace=True)
gname = f.getBestCmap()[ord("e")]
gset = f.getGlyphSet()

bp = BoundsPen(gset)
gset[gname].draw(bp)
gxMin, gyMin, gxMax, gyMax = bp.bounds
gw, gh = gxMax - gxMin, gyMax - gyMin


def path_d(height_frac):
    """The glyph baked into canvas coords, centered, at height_frac of the canvas."""
    s = (SIZE * height_frac) / gh
    tx = (SIZE - gw * s) / 2.0
    ty = (SIZE - gh * s) / 2.0
    # x' = s*x + (tx - s*gxMin);  y' = -s*y + (ty + s*gyMax)   (flip y for SVG)
    xform = (s, 0, 0, -s, tx - s * gxMin, ty + s * gyMax)
    pen = SVGPathPen(gset)
    gset[gname].draw(TransformPen(pen, xform))
    return pen.getCommands(), s


def svg(body):
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {SIZE:g} {SIZE:g}" '
        f'width="{SIZE:g}" height="{SIZE:g}">{body}</svg>\n'
    )


def write(name, body):
    open(os.path.join(HERE, f"ensemble-{name}.svg"), "w").write(svg(body))


d_icon, _ = path_d(0.52)
rounded = f'<rect width="{SIZE:g}" height="{SIZE:g}" rx="{RX:g}" fill="{MINT}"/>'
square = f'<rect width="{SIZE:g}" height="{SIZE:g}" fill="{MINT}"/>'

# app-icon variants: e centered on a mint rounded square
write("icon-mint-black", rounded + f'<path d="{d_icon}" fill="{INK}"/>')
write("icon-mint-white", rounded + f'<path d="{d_icon}" fill="{WHITE}"/>')
# full-bleed square (no rounding) — for apple-touch / PWA maskable, OS adds shape
write("icon-mint-black-square", square + f'<path d="{d_icon}" fill="{INK}"/>')

# large standalone: white e with an ink keyline, transparent background
d_out, _ = path_d(0.72)
write(
    "e-outline",
    f'<path d="{d_out}" fill="{WHITE}" stroke="{INK}" stroke-width="22" '
    f'stroke-linejoin="round" paint-order="stroke"/>',
)

print(f"glyph '{gname}' bounds=({gxMin:.0f},{gyMin:.0f},{gxMax:.0f},{gyMax:.0f})")
print("wrote:", ", ".join(n for n in sorted(os.listdir(HERE)) if n.endswith(".svg")))
