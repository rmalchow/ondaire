#!/usr/bin/env python3
"""Generate amp.kicad_sym — the project-local symbols that are not in the stock
KiCad libraries: the TPA3116D2 class-D amp and the two ESP32-S3 dev-board
sockets (Super Mini + Waveshare Zero). Everything else in the design comes from
the stock symbol libs and is referenced by lib_id at schematic-generation time.

Run from esp32/amp/:  python3 gen_symbols.py
"""

HDR = '''(kicad_symbol_lib
\t(version 20251024)
\t(generator "ensemble-amp-gen")
\t(generator_version "10.0")
'''

def prop(name, value, y, hide=False, x=0.0):
    h = " hide" if hide else ""
    return (f'\t\t(property "{name}" "{value}"\n'
            f'\t\t\t(at {x} {y} 0)\n'
            f'\t\t\t(effects\n\t\t\t\t(font (size 1.27 1.27))' + (h and "\n\t\t\t\thide") +
            f'\n\t\t\t)\n\t\t)\n')

def pin(etype, x, y, rot, name, number, length=2.54):
    return (f'\t\t\t(pin {etype} line\n'
            f'\t\t\t\t(at {x} {y} {rot})\n'
            f'\t\t\t\t(length {length})\n'
            f'\t\t\t\t(name "{name}" (effects (font (size 1.016 1.016))))\n'
            f'\t\t\t\t(number "{number}" (effects (font (size 1.016 1.016))))\n'
            f'\t\t\t)\n')

def rect(x1, y1, x2, y2):
    return (f'\t\t(rectangle\n\t\t\t(start {x1} {y1})\n\t\t\t(end {x2} {y2})\n'
            f'\t\t\t(stroke (width 0.254) (type default))\n'
            f'\t\t\t(fill (type background))\n\t\t)\n')

def symbol(name, footprint, datasheet, descr, pins, half_w, top, bot):
    """pins: list of (etype, side, name, number) placed top->bottom per side.
    side in {L,R}. Body rect from -half_w..half_w, top..bot."""
    s = f'\t(symbol "{name}"\n'
    s += '\t\t(pin_names (offset 1.016))\n\t\t(exclude_from_sim no)\n\t\t(in_bom yes)\n\t\t(on_board yes)\n'
    s += prop("Reference", "U", top + 2.54)
    s += prop("Value", name, bot - 2.54)
    s += prop("Footprint", footprint, 0, hide=True)
    s += prop("Datasheet", datasheet, 0, hide=True)
    s += prop("Description", descr, 0, hide=True)
    # graphic unit
    s += f'\t\t(symbol "{name}_0_1"\n' + rect(-half_w, top, half_w, bot) + '\t\t)\n'
    # pin unit
    s += f'\t\t(symbol "{name}_1_1"\n'
    left = [p for p in pins if p[1] == 'L']
    right = [p for p in pins if p[1] == 'R']
    bottom = [p for p in pins if p[1] == 'B']
    def lay(seq, side):
        out = ''
        n = len(seq)
        span = top - 2.54 - (bot + 2.54)
        step = span / max(n - 1, 1)
        for i, (etype, _, nm, num) in enumerate(seq):
            y = (top - 2.54) - i * step
            y = round(y / 1.27) * 1.27
            if side == 'L':
                out += pin(etype, -half_w - 2.54, y, 0, nm, num)
            else:
                out += pin(etype, half_w + 2.54, y, 180, nm, num)
        return out
    s += lay(left, 'L')
    s += lay(right, 'R')
    # bottom pins (e.g. PowerPAD) spread along the bottom edge
    for i, (etype, _, nm, num) in enumerate(bottom):
        x = -half_w + 2.54 + i * 2.54
        s += pin(etype, x, bot - 2.54, 90, nm, num)
    s += '\t\t)\n\t)\n'
    return s


# ---- TPA3116D2 (32-pin HTSSOP + PowerPAD) — pin map per TI SLOS708G ----
TPA_PINS = [
    # left side, pins 1..16 top->bottom
    ('input',     'L', 'MODSEL',   '1'),
    ('input',     'L', 'SDZ',      '2'),
    ('open_collector', 'L', 'FAULTZ', '3'),
    ('input',     'L', 'RINP',     '4'),
    ('input',     'L', 'RINN',     '5'),
    ('input',     'L', 'PLIMIT',   '6'),
    ('power_out', 'L', 'GVDD',     '7'),
    ('input',     'L', 'GAIN/SLV', '8'),
    ('power_in',  'L', 'GND',      '9'),
    ('input',     'L', 'LINP',     '10'),
    ('input',     'L', 'LINN',     '11'),
    ('input',     'L', 'MUTE',     '12'),
    ('input',     'L', 'AM2',      '13'),
    ('input',     'L', 'AM1',      '14'),
    ('input',     'L', 'AM0',      '15'),
    ('bidirectional', 'L', 'SYNC', '16'),
    # right side, pins 17..32 top->bottom
    ('power_in',  'R', 'AVCC',  '17'),
    ('power_in',  'R', 'PVCC',  '18'),
    ('power_in',  'R', 'PVCC',  '19'),
    ('passive',   'R', 'BSNL',  '20'),
    ('output',    'R', 'OUTNL', '21'),
    ('power_in',  'R', 'GND',   '22'),
    ('output',    'R', 'OUTPL', '23'),
    ('passive',   'R', 'BSPL',  '24'),
    ('power_in',  'R', 'GND',   '25'),
    ('passive',   'R', 'BSNR',  '26'),
    ('output',    'R', 'OUTNR', '27'),
    ('power_in',  'R', 'GND',   '28'),
    ('output',    'R', 'OUTPR', '29'),
    ('passive',   'R', 'BSPR',  '30'),
    ('power_in',  'R', 'PVCC',  '31'),
    ('power_in',  'R', 'PVCC',  '32'),
    ('power_in',  'B', 'PowerPAD', '33'),
]

# ---- ESP32-S3 dev-board sockets. Logical pins = the carrier signals only.
# Pin "numbers" map to the silkscreen GPIO so the netlist reads clearly; the
# footprint maps these to physical header pads (verify row spacing in layout).
def esp_pins():
    return [
        # Two sockets sit on the same nets (populate one), so signal pins are
        # 'bidirectional' — avoids spurious output/output ERC conflicts and lets
        # each socket act as a driver for the nets it feeds.
        ('power_in', 'L', '5V',   '5V'),
        ('passive',  'L', '3V3',  '3V3'),
        ('power_in', 'L', 'GND',  'GND'),
        ('bidirectional', 'L', 'IO5_BCK',  '5'),
        ('bidirectional', 'L', 'IO6_LRCK', '6'),
        ('bidirectional', 'L', 'IO7_DIN',  '7'),
        ('bidirectional', 'R', 'IO8_SDA',  '8'),
        ('bidirectional', 'R', 'IO18_SCL', '18'),
        ('bidirectional', 'R', 'IO15_ENCA','15'),
        ('bidirectional', 'R', 'IO16_ENCB','16'),
        ('bidirectional', 'R', 'IO17_ENCSW','17'),
        ('bidirectional', 'R', 'IO1_AMPSD','1'),
        ('bidirectional', 'R', 'IO2_PG',   '2'),
        ('bidirectional', 'R', 'IO4_FAULT','4'),
    ]

out = HDR
out += symbol(
    "TPA3116D2",
    "Package_SO:HTSSOP-32-1EP_6.1x11mm_P0.65mm_EP5.2x11mm_Mask4.11x4.36mm_ThermalVias",
    "https://www.ti.com/lit/ds/symlink/tpa3116d2.pdf",
    "30W/50W filter-free class-D amplifier, HTSSOP-32 PowerPAD (PBTL mono here)",
    TPA_PINS, half_w=12.7, top=22.86, bot=-22.86)
out += symbol(
    "ESP32-S3-SuperMini-Socket",
    "amp:ESP32S3_SuperMini_Socket",
    "https://www.espressif.com/en/products/socs/esp32-s3",
    "Socket for ESP32-S3 Super Mini dev board (carrier signals only)",
    esp_pins(), half_w=10.16, top=12.7, bot=-12.7)
out += symbol(
    "ESP32-S3-Zero-Socket",
    "amp:ESP32S3_Zero_Socket",
    "https://www.waveshare.com/wiki/ESP32-S3-Zero",
    "Socket for Waveshare ESP32-S3-Zero dev board (carrier signals only)",
    esp_pins(), half_w=10.16, top=12.7, bot=-12.7)
out += ')\n'

with open("amp.kicad_sym", "w") as f:
    f.write(out)
print("wrote amp.kicad_sym")
