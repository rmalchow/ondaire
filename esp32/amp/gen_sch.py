#!/usr/bin/env python3
"""Generate amp.kicad_sch from a declarative netlist.

Style: every component is placed on a grid and every pin carries a global label
naming its net. Same-named labels are electrically one net, so this captures the
complete, ERC-checkable netlist without hand-routed wires — a connectivity
schematic meant as the starting point for board layout in KiCad. Re-run after
editing NETS/PARTS to regenerate.

    python3 gen_sch.py && kicad-cli sch erc amp.kicad_sch

Pin coordinates come from the real symbol definitions (stock libs + amp.kicad_sym),
so labels land exactly on each pin's connection point.
"""
import os, sys, uuid, re

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ksym

SYMDIR = "/usr/share/kicad/symbols"

# lib_id -> (source file, symbol name, extends-base or None)
LIBSRC = {
    "Device:R":           (f"{SYMDIR}/Device.kicad_sym", "R", None),
    "Device:C":           (f"{SYMDIR}/Device.kicad_sym", "C", None),
    "Device:C_Polarized": (f"{SYMDIR}/Device.kicad_sym", "C_Polarized", None),
    "Device:L":           (f"{SYMDIR}/Device.kicad_sym", "L", None),
    "Device:D_Schottky":  (f"{SYMDIR}/Device.kicad_sym", "D_Schottky", None),
    "Device:LED":         (f"{SYMDIR}/Device.kicad_sym", "LED", None),
    "Connector:USB_C_Receptacle_USB2.0_16P": (f"{SYMDIR}/Connector.kicad_sym", "USB_C_Receptacle_USB2.0_16P", None),
    "Connector_Generic:Conn_01x02": (f"{SYMDIR}/Connector_Generic.kicad_sym", "Conn_01x02", None),
    "Connector_Generic:Conn_01x04": (f"{SYMDIR}/Connector_Generic.kicad_sym", "Conn_01x04", None),
    "Connector_Generic:Conn_01x05": (f"{SYMDIR}/Connector_Generic.kicad_sym", "Conn_01x05", None),
    "Interface_USB:CH224K": (f"{SYMDIR}/Interface_USB.kicad_sym", "CH224K", None),
    "Audio:PCM5102A":     (f"{SYMDIR}/Audio.kicad_sym", "PCM5102A", "PCM5100"),
    "Regulator_Switching:LM2596S-5": (f"{SYMDIR}/Regulator_Switching.kicad_sym", "LM2596S-5", "LM2596S-12"),
    "Regulator_Linear:AMS1117-3.3":  (f"{SYMDIR}/Regulator_Linear.kicad_sym", "AMS1117-3.3", "AP1117-15"),
    "power:PWR_FLAG":     (f"{SYMDIR}/power.kicad_sym", "PWR_FLAG", None),
    "amp:TPA3116D2":                 ("amp.kicad_sym", "TPA3116D2", None),
    "amp:ESP32-S3-SuperMini-Socket": ("amp.kicad_sym", "ESP32-S3-SuperMini-Socket", None),
    "amp:ESP32-S3-Zero-Socket":      ("amp.kicad_sym", "ESP32-S3-Zero-Socket", None),
}

FP = {  # footprints (verify in layout; flagged in README where uncertain)
    "R": "Resistor_SMD:R_0805_2012Metric",
    "C": "Capacitor_SMD:C_0805_2012Metric",
    "C_Polarized": "Capacitor_SMD:CP_Elec_8x10.5",
    "L": "Inductor_SMD:L_12x12mm_H6mm",       # buck (33uH)
    "L_OUT": "Inductor_SMD:L_7.3x7.3_H4.5",   # class-D output filter (10uH)
    "D_Schottky": "Diode_SMD:D_SMB",
    "LED": "LED_SMD:LED_0805_2012Metric",
    "USB_C": "Connector_USB:USB_C_Receptacle_GCT_USB4085",
    "Conn02": "TerminalBlock_Phoenix:TerminalBlock_Phoenix_MKDS-1,5-2_1x02_P5.00mm_Horizontal",
    "Conn04": "Connector_JST:JST_PH_B4B-PH-K_1x04_P2.00mm_Vertical",
    "Conn05": "Connector_JST:JST_PH_B5B-PH-K_1x05_P2.00mm_Vertical",
    "CH224K": "Package_SO:MSOP-10-1EP_3x3mm_P0.5mm_EP1.73x1.98mm",
    "PCM5102A": "Package_SO:TSSOP-20_4.4x6.5mm_P0.65mm",
    "LM2596S-5": "Package_TO_SOT_SMD:TO-263-5_TabPin3",
    "AMS1117-3.3": "Package_TO_SOT_SMD:SOT-223-3_TabPin2",
    "TPA": "Package_SO:HTSSOP-32-1EP_6.1x11mm_P0.65mm_EP5.2x11mm_Mask4.11x4.36mm_ThermalVias",
    "ESP_SM": "amp:ESP32S3_SuperMini_Socket",
    "ESP_ZERO": "amp:ESP32S3_Zero_Socket",
}

# ---- PARTS: ref -> (lib_id, value, footprint) ----
PARTS = {}
def add(ref, lib, val, fp):
    PARTS[ref] = (lib, val, fp)

# Power input + USB-PD
add("J1", "Connector:USB_C_Receptacle_USB2.0_16P", "USB-C PD in", FP["USB_C"])
add("U1", "Interface_USB:CH224K", "CH224K", FP["CH224K"])
# CH224K single-resistor PD voltage select: CFG1->GND resistor chooses voltage,
# CFG2/CFG3 left unconnected. R1 UNPOPULATED (NC) => 20V (per WCH CH224K DS 5.2.1).
# Populate R1 = 6.8k->9V, 24k->12V, 56k->15V if a lower rail is ever wanted.
add("R1", "Device:R", "DNP (NC=20V; 6.8k/24k/56k=9/12/15V)", FP["R"])
add("C1", "Device:C", "1uF", FP["C"])      # CH224K VDD decoupling (+ series R to VBUS)
add("R3", "Device:R", "10k", FP["R"])      # PG pull-up to 3V3
add("CB1", "Device:C_Polarized", "470uF/35V", FP["C_Polarized"])
add("CB2", "Device:C_Polarized", "470uF/35V", FP["C_Polarized"])
add("C2", "Device:C", "100nF", FP["C"])    # 20V rail HF
add("R4", "Device:R", "2.2k", FP["R"])     # power LED series (+5V)
add("D2", "Device:LED", "PWR", FP["LED"])  # power-on LED on +5V

# Buck 20V -> 5V  (LM2596S-5.0)
add("U2", "Regulator_Switching:LM2596S-5", "LM2596S-5.0", FP["LM2596S-5"])
add("CI1", "Device:C_Polarized", "220uF/35V", FP["C_Polarized"])
add("C3", "Device:C", "100nF", FP["C"])
add("L1", "Device:L", "33uH/3A", FP["L"])
add("D1", "Device:D_Schottky", "SS54", FP["D_Schottky"])
add("CO1", "Device:C_Polarized", "330uF/16V", FP["C_Polarized"])
add("C4", "Device:C", "100nF", FP["C"])

# LDO 5V -> 3.3V (AMS1117-3.3) for clean DAC/logic rail
add("U3", "Regulator_Linear:AMS1117-3.3", "AMS1117-3.3", FP["AMS1117-3.3"])
add("C5", "Device:C", "10uF", FP["C"])
add("C6", "Device:C", "22uF", FP["C"])
add("C7", "Device:C", "100nF", FP["C"])

# DAC (PCM5102A, MCLK-less)
add("U4", "Audio:PCM5102A", "PCM5102A", FP["PCM5102A"])
add("C8", "Device:C", "100nF", FP["C"])    # DVDD
add("C9", "Device:C", "100nF", FP["C"])    # AVDD
add("C10", "Device:C", "10uF", FP["C"])    # AVDD bulk
add("C11", "Device:C", "100nF", FP["C"])   # CPVDD
add("C12", "Device:C", "2.2uF", FP["C"])   # charge-pump flying cap
add("C13", "Device:C", "1uF", FP["C"])     # VNEG
add("C14", "Device:C", "1uF", FP["C"])     # LDOO
add("R5", "Device:R", "10k", FP["R"])      # XSMT pull-up (unmute)

# Amp input network (L+R passive sum -> single-ended into RINP)
add("R6", "Device:R", "10k", FP["R"])      # sum L
add("R7", "Device:R", "10k", FP["R"])      # sum R
add("C15", "Device:C", "1uF", FP["C"])     # AC-couple to RINP
add("C16", "Device:C", "1uF", FP["C"])     # balance to RINN
add("R17", "Device:R", "DNP (atten/downmix)", FP["R"])  # optional MIXIN->GND shunt

# Amp (TPA3116D2 PBTL mono)
add("U5", "amp:TPA3116D2", "TPA3116D2", FP["TPA"])
add("R8", "Device:R", "3.3", FP["R"])      # AVCC RC filter series
add("C17", "Device:C", "1uF", FP["C"])     # AVCC
add("C18", "Device:C", "100nF", FP["C"])   # AVCC HF
add("C19", "Device:C", "1uF", FP["C"])     # GVDD decoupling
add("R9", "Device:R", "5.6k", FP["R"])     # GAIN/SLV (20 dB, master) [GVDD->GAIN]
add("R10", "Device:R", "100k", FP["R"])    # SDZ pull-down (amp off at boot)
add("R11", "Device:R", "10k", FP["R"])     # FAULTZ pull-up
add("CP1", "Device:C", "100nF", FP["C"])   # PVCC HF
add("CP2", "Device:C", "1nF", FP["C"])     # PVCC snubber
add("CP3", "Device:C_Polarized", "220uF/35V", FP["C_Polarized"])  # PVCC bulk @ amp
add("CBR1", "Device:C", "220nF", FP["C"])  # BSPR
add("CBR2", "Device:C", "220nF", FP["C"])  # BSNR
add("CBL1", "Device:C", "220nF", FP["C"])  # BSPL
add("CBL2", "Device:C", "220nF", FP["C"])  # BSNL
add("LR1", "Device:L", "10uH/4A", FP["L_OUT"]) # OUTPR
add("LR2", "Device:L", "10uH/4A", FP["L_OUT"]) # OUTNR
add("LL1", "Device:L", "10uH/4A", FP["L_OUT"]) # OUTPL
add("LL2", "Device:L", "10uH/4A", FP["L_OUT"]) # OUTNL
add("CF1", "Device:C", "1uF/63V", FP["C"]) # output filter across speaker
add("J2", "Connector_Generic:Conn_01x02", "SPEAKER", FP["Conn02"])

# OLED + encoder JSTs
add("J3", "Connector_Generic:Conn_01x04", "OLED I2C", FP["Conn04"])
add("R12", "Device:R", "4.7k", FP["R"])    # SDA pull-up
add("R13", "Device:R", "4.7k", FP["R"])    # SCL pull-up
add("J4", "Connector_Generic:Conn_01x05", "ENCODER", FP["Conn05"])
add("R14", "Device:R", "10k", FP["R"])     # ENC_A pull-up
add("R15", "Device:R", "10k", FP["R"])     # ENC_B pull-up
add("R16", "Device:R", "10k", FP["R"])     # ENC_SW pull-up
add("C20", "Device:C", "100nF", FP["C"])   # ENC_A debounce
add("C21", "Device:C", "100nF", FP["C"])   # ENC_B debounce
add("C22", "Device:C", "100nF", FP["C"])   # ENC_SW debounce

# ESP32-S3 sockets (populate one)
add("U6", "amp:ESP32-S3-SuperMini-Socket", "ESP32-S3 Super Mini", FP["ESP_SM"])
add("U7", "amp:ESP32-S3-Zero-Socket", "ESP32-S3 Zero", FP["ESP_ZERO"])

# Power flags. +5V and +3V3 are driven by their regulators' power-output pins,
# so they need none. Flag the externally/locally sourced rails that ERC can't
# otherwise see a driver for: +20V (from USB), GND, CH224K VDD (internal LDO),
# and the amp AVCC (fed through an RC from +20V).
add("PWR1", "power:PWR_FLAG", "PWR_FLAG", "")   # +20V
add("PWR2", "power:PWR_FLAG", "PWR_FLAG", "")   # CH_VDD
add("PWR3", "power:PWR_FLAG", "PWR_FLAG", "")   # AMP_AVCC
add("PWR4", "power:PWR_FLAG", "PWR_FLAG", "")   # GND
add("PWR5", "power:PWR_FLAG", "PWR_FLAG", "")   # +5V (post-inductor, no direct power-out pin)

# ---- NETS: name -> list of "REF.PINNUMBER" ----
NETS = {
    "+20V": ["J1.A4", "J1.A9", "J1.B4", "J1.B9", "U1.8",
             "CB1.1", "CB2.1", "C2.1", "U2.1", "CI1.1", "C3.1", "R4.1",
             "U5.18", "U5.19", "U5.31", "U5.32", "R8.1", "CP1.1", "CP2.1", "CP3.1",
             "PWR1.1"],
    "+5V": ["U2.2_FBOUT", "L1.2", "CO1.1", "C4.1", "U3.3", "C5.1",
            "U6.5V", "U7.5V", "PWR5.1"],   # fed through L1 from SW node
    "+3V3": ["U3.2", "C6.1", "C7.1",
             "U4.20", "U4.8", "U4.1", "C8.1", "C9.1", "C10.1", "C11.1",
             "R5.1", "R11.1", "R3.1", "R12.1", "R13.1", "R14.1", "R15.1", "R16.1",
             "J3.2", "J4.1"],     # driven by AMS1117 VO (power output)
    "GND": ["J1.A1", "J1.B1", "J1.A12", "J1.B12", "J1.SH",
            "U1.11", "R1.2", "C1.2", "CB1.2", "CB2.2", "C2.2", "D2.1",
            "CI1.2", "C3.2", "D1.1", "CO1.2", "C4.2", "U3.1", "C5.2", "C6.2", "C7.2",
            "U4.3", "U4.9", "U4.19", "C8.2", "C9.2", "C10.2", "C11.2", "C13.2", "C14.2",
            "C16.1", "U5.9", "U5.22", "U5.25", "U5.28", "U5.33", "U5.10", "U5.11",
            "U5.1", "U5.12", "U5.13", "U5.14", "U5.15", "C17.2", "C18.2", "CP1.2", "CP2.2", "CP3.2",
            "J2.2_n", "J3.1", "J4.2", "C20.2", "C21.2", "C22.2", "R17.2",
            "U6.GND", "U7.GND", "PWR4.1"],

    # USB-PD control
    "USB_CC1": ["J1.A5", "U1.7"],
    "USB_CC2": ["J1.B5", "U1.6"],
    "USB_DP":  ["J1.A6", "J1.B6", "U1.4"],
    "USB_DM":  ["J1.A7", "J1.B7", "U1.5"],
    "CH_VDD":  ["U1.1", "C1.1", "PWR2.1"],
    "CH_CFG1": ["U1.9", "R1.1"],   # CFG2 (U1.2) & CFG3 (U1.3) left NC -> single-resistor mode
    "PG":      ["U1.10", "R3.2", "U6.2", "U7.2"],

    # Buck switch node
    "SW5": ["U2.2", "L1.1", "D1.2"],

    # power LED
    "PWR_LED": ["R4.2", "D2.2"],

    # I2S DAC <- ESP
    "I2S_BCK":  ["U4.13", "U6.5", "U7.5"],
    "I2S_LRCK": ["U4.15", "U6.6", "U7.6"],
    "I2S_DOUT": ["U4.14", "U6.7", "U7.7"],

    # DAC config / charge pump
    "DAC_SCK":  ["U4.12"],   # -> GND (MCLK-less)
    "DAC_FMT":  ["U4.16"],   # -> GND
    "DAC_FLT":  ["U4.11"],   # -> GND
    "DAC_DEMP": ["U4.10"],   # -> GND
    "DAC_XSMT": ["U4.17", "R5.2"],
    "DAC_CPP":  ["U4.2", "C12.1"],
    "DAC_CPM":  ["U4.4", "C12.2"],
    "DAC_VNEG": ["U4.5", "C13.1"],
    "DAC_LDOO": ["U4.18", "C14.1"],

    # DAC outputs -> amp input network
    "DAC_OUTL": ["U4.6", "R6.1"],
    "DAC_OUTR": ["U4.7", "R7.1"],
    "MIXIN":    ["R6.2", "R7.2", "C15.1", "R17.1"],
    "AMP_INP":  ["C15.2", "U5.4"],
    "AMP_INN":  ["C16.2", "U5.5"],

    # amp control / supplies
    "AMP_AVCC": ["R8.2", "C17.1", "C18.1", "U5.17", "PWR3.1"],
    "AMP_GVDD": ["U5.7", "C19.1", "U5.6", "R9.1"],   # PLIMIT(6) tied to GVDD
    "AMP_GAIN": ["R9.2", "U5.8"],
    "AMP_SD":   ["U5.2", "R10.2", "U6.1", "U7.1"],
    "AMP_FAULT":["U5.3", "R11.2", "U6.4", "U7.4"],

    # bootstrap caps
    "BS_PR": ["U5.30", "CBR1.1"],
    "BS_NR": ["U5.26", "CBR2.1"],
    "BS_PL": ["U5.24", "CBL1.1"],
    "BS_NL": ["U5.20", "CBL2.1"],

    # amp outputs (pre-filter)
    "OUT_PR": ["U5.29", "CBR1.2", "LR1.1"],
    "OUT_NR": ["U5.27", "CBR2.2", "LR2.1"],
    "OUT_PL": ["U5.23", "CBL1.2", "LL1.1"],
    "OUT_NL": ["U5.21", "CBL2.2", "LL2.1"],

    # speaker (post LC, PBTL paralleled)
    "SPK_P": ["LR1.2", "LR2.2", "CF1.1", "J2.1"],
    "SPK_N": ["LL1.2", "LL2.2", "CF1.2", "J2.2"],

    # I2C OLED
    "I2C_SDA": ["J3.4", "R12.2", "U6.8", "U7.8"],
    "I2C_SCL": ["J3.3", "R13.2", "U6.18", "U7.18"],

    # rotary encoder
    "ENC_A":  ["J4.3", "R14.2", "C20.1", "U6.15", "U7.15"],
    "ENC_B":  ["J4.4", "R15.2", "C21.1", "U6.16", "U7.16"],
    "ENC_SW": ["J4.5", "R16.2", "C22.1", "U6.17", "U7.17"],
}

# nets that are simply "this pin -> GND"
for n in ["DAC_SCK", "DAC_FMT", "DAC_FLT", "DAC_DEMP"]:
    NETS["GND"].extend(NETS[n])
    del_keys = NETS.pop(n)
# MODSEL(1) already to GND above; AM0/1/2 to GND above; LINP/LINN to GND above.

# Special: LM2596S-5 OUT pin(2) is also the switch node AND FB(4) ties to +5V.
# Fix the +5V net's placeholder entries.
NETS["+5V"] = [x for x in NETS["+5V"] if x != "U2.2_FBOUT"]
NETS["+5V"].append("U2.4")          # FB -> +5V (fixed 5V part)
# fix GND speaker placeholder
NETS["GND"] = [("J2.2" if x == "J2.2_n" else x) for x in NETS["GND"]]
# LM2596 ON/OFF (pin5) -> GND (enabled, active low)
NETS["GND"].append("U2.5")

# ---------------------------------------------------------------------------
# Build symbol pin-coordinate cache
ROOTS = {}
def root_for(path):
    if path not in ROOTS:
        ROOTS[path] = ksym.load(path)
    return ROOTS[path]

PINCACHE = {}
def pins_for(lib_id):
    if lib_id not in PINCACHE:
        path, name, _ = LIBSRC[lib_id]
        ps = ksym.pins_of(root_for(path), name)
        PINCACHE[lib_id] = {p['number']: p for p in ps}
    return PINCACHE[lib_id]

def embed_symbol(lib_id):
    path, name, base = LIBSRC[lib_id]
    nick = lib_id.split(":")[0]
    if base:
        text = ksym.symbol_block_text(path, base)
        text = text.replace(base, name)
        text = text.replace(f'(symbol "{name}"', f'(symbol "{nick}:{name}"', 1)
    else:
        text = ksym.symbol_block_text(path, name)
        text = text.replace(f'(symbol "{name}"', f'(symbol "{nick}:{name}"', 1)
    # indent one level deeper for the lib_symbols section
    return "\n".join("\t" + ln if ln.strip() else ln for ln in text.splitlines())

def U():
    return str(uuid.uuid4())

# placement grid
order = list(PARTS.keys())
COLS = 9
DX, DY = 63.5, 76.2
X0, Y0 = 50.8, 50.8
pos = {}
for i, ref in enumerate(order):
    c, r = i % COLS, i // COLS
    pos[ref] = (X0 + c * DX, Y0 + r * DY)

def pin_sheet_xy(ref, pinnum):
    lib_id = PARTS[ref][0]
    p = pins_for(lib_id)[pinnum]
    X, Y = pos[ref]
    return round(X + p['x'], 4), round(Y - p['y'], 4)

# map (ref,pin) -> net
pin2net = {}
for net, members in NETS.items():
    for m in members:
        ref, pinnum = m.split(".", 1)
        pin2net[(ref, pinnum)] = net

# ---- emit ----
out = []
out.append('(kicad_sch')
out.append('\t(version 20251024)')
out.append('\t(generator "ensemble-amp-gen")')
out.append('\t(generator_version "10.0")')
ROOT_UUID = "a3c1e0d2-0001-4a00-8000-656e73656d62"  # stable, referenced by amp.kicad_pro
out.append(f'\t(uuid "{ROOT_UUID}")')
out.append('\t(paper "A1")')
# lib_symbols
out.append('\t(lib_symbols')
for lib_id in LIBSRC:
    if any(p[0] == lib_id for p in PARTS.values()):
        out.append(embed_symbol(lib_id))
out.append('\t)')

# component instances
for ref in order:
    lib_id, val, fp = PARTS[ref]
    X, Y = pos[ref]
    u = U()
    out.append(f'\t(symbol')
    out.append(f'\t\t(lib_id "{lib_id}")')
    out.append(f'\t\t(at {X} {Y} 0)')
    out.append(f'\t\t(unit 1)')
    out.append(f'\t\t(exclude_from_sim no)\n\t\t(in_bom yes)\n\t\t(on_board yes)\n\t\t(dnp no)')
    out.append(f'\t\t(uuid "{u}")')
    out.append(f'\t\t(property "Reference" "{ref}" (at {X} {Y-3} 0) (effects (font (size 1.27 1.27))))')
    out.append(f'\t\t(property "Value" "{val}" (at {X} {Y+3} 0) (effects (font (size 1.27 1.27))))')
    out.append(f'\t\t(property "Footprint" "{fp}" (at {X} {Y} 0) (effects (font (size 1.27 1.27)) hide))')
    out.append(f'\t\t(instances')
    out.append(f'\t\t\t(project "amp"')
    out.append(f'\t\t\t\t(path "/" (reference "{ref}") (unit 1))')
    out.append(f'\t\t\t)\n\t\t)')
    out.append(f'\t)')

# global labels on connected pins; no_connect on the rest
placed_label = set()   # (net, x, y)
connected_xy = set()   # (ref, x, y) that already carry a connection
for ref in order:
    lib_id = PARTS[ref][0]
    for pinnum, p in pins_for(lib_id).items():
        net = pin2net.get((ref, pinnum))
        if net:
            connected_xy.add((ref, *pin_sheet_xy(ref, pinnum)))

for ref in order:
    lib_id = PARTS[ref][0]
    for pinnum, p in pins_for(lib_id).items():
        x, y = pin_sheet_xy(ref, pinnum)
        net = pin2net.get((ref, pinnum))
        if net:
            key = (net, x, y)
            if key in placed_label:
                continue
            placed_label.add(key)
            px = p['x']
            if abs(px) >= abs(p['y']):
                ang, just = (0, "left") if px > 0 else (180, "right")
            else:
                ang, just = (90, "left") if p['y'] > 0 else (270, "right")
            out.append(f'\t(global_label "{net}" (shape bidirectional) (at {x} {y} {ang}) '
                       f'(effects (font (size 1.27 1.27)) (justify {just})) (uuid "{U()}"))')
        else:
            if (ref, x, y) in connected_xy:
                continue   # stacked pin already connected
            out.append(f'\t(no_connect (at {x} {y}) (uuid "{U()}"))')

out.append('\t(sheet_instances\n\t\t(path "/" (page "1"))\n\t)')
out.append(')')

with open("amp.kicad_sch", "w") as f:
    f.write("\n".join(out) + "\n")
print(f"wrote amp.kicad_sch  ({len(PARTS)} parts, {len(NETS)} nets)")
