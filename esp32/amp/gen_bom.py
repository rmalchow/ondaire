#!/usr/bin/env python3
"""Write bom.csv grouped by value+footprint from the schematic's PARTS table.

LCSC codes are best-effort starting points for Aisler/JLC-style assembly and
MUST be verified against the live catalogue before ordering. Passives are
jellybean 0805 unless noted; pick voltage/temperature ratings per the Value.
"""
import importlib, csv, sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
g = importlib.import_module("gen_sch")
PARTS = g.PARTS

# value -> (LCSC, note). Verify all before ordering.
LCSC = {
    "TPA3116D2":   ("C51591",  "TPA3116D2DADR, HTSSOP-32 PowerPAD"),
    "PCM5102A":    ("C107916", "PCM5102APWR, TSSOP-20"),
    "CH224K":      ("C970725", "USB-PD sink, ESSOP-10"),
    "LM2596S-5.0": ("C347421", "fixed 5V buck, TO-263-5"),
    "AMS1117-3.3": ("C6186",   "SOT-223 LDO"),
    "SS54":        ("C22452",  "5A 40V Schottky, SMB"),
    "USB-C PD in": ("",        "16-pin USB-C receptacle, GCT USB4085 or equiv (power+CC+D)"),
    "USB-C":       ("",        ""),
}

groups = {}
for ref, (lib, val, fp) in PARTS.items():
    if lib == "power:PWR_FLAG":
        continue
    key = (val, fp)
    groups.setdefault(key, []).append(ref)

rows = []
for (val, fp), refs in sorted(groups.items(), key=lambda kv: kv[0][0]):
    lcsc, note = LCSC.get(val, ("", ""))
    rows.append({
        "Refs": ",".join(sorted(refs, key=lambda r: (r[:2], r))),
        "Qty": len(refs),
        "Value": val,
        "Footprint": fp.split(":")[-1] if ":" in fp else fp,
        "LCSC": lcsc,
        "Notes": note,
    })

with open("bom.csv", "w", newline="") as f:
    w = csv.DictWriter(f, fieldnames=["Refs", "Qty", "Value", "Footprint", "LCSC", "Notes"])
    w.writeheader()
    for r in rows:
        w.writerow(r)

print(f"wrote bom.csv ({len(rows)} line items, {sum(r['Qty'] for r in rows)} placed parts)")
