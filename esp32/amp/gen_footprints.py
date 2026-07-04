#!/usr/bin/env python3
"""Generate the project-local footprint library amp.pretty.

Currently just the two ESP32-S3 dev-board sockets. Both the Super Mini and the
Waveshare Zero break out every signal this carrier uses (5V/GND/3V3 + GPIO
1,2,4,5,6,7,8,15,16,17,18) on their single "GPIO1-18" long edge, so each socket
is a 1x14 female-header strip tapping that edge.

IMPORTANT: these are land-pattern PLACEHOLDERS. Super Mini clones vary in length,
row spacing, and exact pad order; the Waveshare Zero is castellated + THT. Before
ordering, measure your actual board and confirm pad order / spacing (and add the
mechanical support header along the opposite edge). Pad "numbers" match the
symbol pin numbers so the netlist stays correct when you adjust geometry.
"""
import os

os.makedirs("amp.pretty", exist_ok=True)

# (pad number == symbol pin number) in physical top->bottom edge order
EDGE = ["5V", "GND", "3V3", "1", "2", "4", "5", "6", "7", "8", "15", "16", "17", "18"]

def footprint(name, descr):
    pitch = 2.54
    n = len(EDGE)
    y0 = -(n - 1) * pitch / 2
    lines = [f'(footprint "{name}"',
             '\t(version 20240108)',
             '\t(generator "ondaire-amp-gen")',
             '\t(layer "F.Cu")',
             f'\t(descr "{descr}")',
             '\t(attr through_hole)']
    for i, num in enumerate(EDGE):
        y = round(y0 + i * pitch, 3)
        shape = "rect" if i == 0 else "circle"
        lines.append(
            f'\t(pad "{num}" thru_hole {shape} (at 0 {y}) (size 1.7 1.7) '
            f'(drill 1.0) (layers "*.Cu" "*.Mask"))')
    # silk outline + label
    top = round(y0 - 1.5, 3); bot = round(y0 + (n - 1) * pitch + 1.5, 3)
    lines += [
        f'\t(fp_line (start -1.6 {top}) (end 1.6 {top}) (stroke (width 0.12) (type solid)) (layer "F.SilkS"))',
        f'\t(fp_line (start -1.6 {bot}) (end 1.6 {bot}) (stroke (width 0.12) (type solid)) (layer "F.SilkS"))',
        f'\t(fp_line (start -1.6 {top}) (end -1.6 {bot}) (stroke (width 0.12) (type solid)) (layer "F.SilkS"))',
        f'\t(fp_line (start 1.6 {top}) (end 1.6 {bot}) (stroke (width 0.12) (type solid)) (layer "F.SilkS"))',
        f'\t(fp_text reference "REF**" (at -3 {top}) (layer "F.SilkS") (effects (font (size 1 1) (thickness 0.15))))',
        f'\t(fp_text value "{name}" (at 3 {bot}) (layer "F.Fab") (effects (font (size 1 1) (thickness 0.15))))',
        ')']
    return "\n".join(lines) + "\n"

specs = {
    "ESP32S3_SuperMini_Socket": "ESP32-S3 Super Mini socket (GPIO1-18 edge) - PLACEHOLDER, verify board geometry",
    "ESP32S3_Zero_Socket": "Waveshare ESP32-S3-Zero socket (GPIO1-18 edge) - PLACEHOLDER, verify board geometry",
}
for name, descr in specs.items():
    with open(f"amp.pretty/{name}.kicad_mod", "w") as f:
        f.write(footprint(name, descr))
    print(f"wrote amp.pretty/{name}.kicad_mod")
