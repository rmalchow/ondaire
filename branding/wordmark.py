#!/usr/bin/env python3
"""Emit the horizontal ensemble wordmark: the mint 'e' badge + "ensemble".

The badge reuses the weight-900 'e'; the word is set in Fraunces 600 (matching
the site header) and converted to outlines, so the SVG needs no font at render
time. Two colourways: dark ink text (light backgrounds) and light text (dark).
"""
import os
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen

HERE = os.path.dirname(os.path.abspath(__file__))
FONT = os.path.join(HERE, "..", "site", "src", "assets", "fonts", "fraunces-wght.woff2")

MINT, INK, LIGHT = "#35e3b3", "#11151a", "#f4f7fa"
WORD = "ensemble"
TRACK = -14        # letter tracking in font units (~ -0.01em, matches header)
WORD_H = 118.0     # target tight height of the word, in canvas units
BADGE_SCALE = 1.16 # badge side relative to word height
E_FRAC = 0.52      # 'e' height inside the badge (same as the app icon)


def load(wght):
    f = TTFont(FONT)
    instantiateVariableFont(f, {"wght": wght}, inplace=True)
    return f


def glyph_name(f, ch):
    return f.getBestCmap()[ord(ch)]


def draw_word(f, pen):
    """Draw WORD into pen in font units (baseline y=0, start x=0). Returns width."""
    gset, hmtx = f.getGlyphSet(), f["hmtx"]
    x = 0
    for ch in WORD:
        name = glyph_name(f, ch)
        gset[name].draw(TransformPen(pen, (1, 0, 0, 1, x, 0)))
        x += hmtx[name][0] + TRACK
    return x - TRACK  # drop trailing track


f900, f600 = load(900), load(600)

# --- badge 'e' (900) tight bounds ---
gsetE = f900.getGlyphSet()
eName = glyph_name(f900, "e")
bpE = BoundsPen(gsetE)
gsetE[eName].draw(bpE)
exMin, eyMin, exMax, eyMax = bpE.bounds
egw, egh = exMax - exMin, eyMax - eyMin

# --- word (600) tight bounds ---
bpW = BoundsPen(f600.getGlyphSet())
draw_word(f600, bpW)
wxMin, wyMin, wxMax, wyMax = bpW.bounds
word_gh = wyMax - wyMin

# --- layout (canvas units) ---
s = WORD_H / word_gh
badge = round(WORD_H * BADGE_SCALE)
gap = round(badge * 0.30)
canvasH = badge
wordX0 = badge + gap
wordY0 = (canvasH - WORD_H) / 2.0
word_w = (wxMax - wxMin) * s
dot_r = round(badge * 0.058)
dot_gap = round(badge * 0.16)
dot_cx = wordX0 + word_w + dot_gap + dot_r
dot_cy = wordY0 + WORD_H / 2.0
canvasW = round(dot_cx + dot_r)

# badge 'e' transform: centre the 'e' in the [0,badge] square, flip y
eS = (badge * E_FRAC) / egh
etx, ety = (badge - egw * eS) / 2.0, (badge - egh * eS) / 2.0
eXform = (eS, 0, 0, -eS, etx - eS * exMin, ety + eS * eyMax)
penE = SVGPathPen(gsetE)
gsetE[eName].draw(TransformPen(penE, eXform))
e_d = penE.getCommands()

# word transform: place tight bbox at (wordX0, wordY0), flip y
wXform = (s, 0, 0, -s, wordX0 - s * wxMin, wordY0 + s * wyMax)
penW = SVGPathPen(f600.getGlyphSet())
draw_word(f600, TransformPen(penW, wXform))
word_d = penW.getCommands()

rx = round(badge * 0.223)
badge_svg = (
    f'<rect width="{badge}" height="{badge}" rx="{rx}" fill="{MINT}"/>'
    f'<path d="{e_d}" fill="{INK}"/>'
)


def write(name, text_fill):
    body = (
        badge_svg
        + f'<path d="{word_d}" fill="{text_fill}"/>'
        + f'<circle cx="{dot_cx:g}" cy="{dot_cy:g}" r="{dot_r}" fill="{MINT}"/>'
    )
    svg = (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {canvasW} {canvasH}" '
        f'width="{canvasW}" height="{canvasH}">{body}</svg>\n'
    )
    open(os.path.join(HERE, name), "w").write(svg)


write("ensemble-wordmark.svg", INK)          # dark text — light backgrounds
write("ensemble-wordmark-inverse.svg", LIGHT)  # light text — dark backgrounds
print(f"wordmark {canvasW}x{canvasH} (badge {badge}, word {word_w:.0f} wide)")
