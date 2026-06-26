#!/usr/bin/env python3
"""Generate amp.kicad_pcb: a placed, ground-poured starting board.

Uses the KiCad pcbnew Python API so footprints come straight from the libraries
with correct geometry. It does:
  - load every footprint and read its real courtyard size
  - shelf-pack the parts into functional REGIONS so courtyards never overlap
  - assign nets from the schematic netlist (gen_sch.pin2net)
  - draw the Edge.Cuts outline
  - pour GND on F.Cu and B.Cu (this "routes" the whole ground net)

It does NOT route the signal/power nets — those remain as a ratsnest for you to
route by hand (see README "PCB layout"). The TPA3116 uses the "_ThermalVias"
footprint, so the PowerPAD is already stitched to the back layer. Re-run after
editing REGIONS/BOARD:

    python3 gen_pcb.py && kicad-cli pcb drc amp.kicad_pcb
"""
import os, sys, importlib
import pcbnew

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
g = importlib.import_module("gen_sch")   # regenerates sch; gives PARTS + pin2net
PARTS, pin2net = g.PARTS, g.pin2net

def MM(x): return pcbnew.FromMM(x)
def V(x, y): return pcbnew.VECTOR2I(MM(x), MM(y))

def fp_dir(footprint):
    lib, name = footprint.split(":")
    d = os.path.join(HERE, "amp.pretty") if lib == "amp" else f"/usr/share/kicad/footprints/{lib}.pretty"
    return d, name

BOARD_W, BOARD_H = 120.0, 95.0

# ---- load every footprint once; record its courtyard (or bbox) size ----
FOOT = {}   # ref -> FOOTPRINT
SIZE = {}   # ref -> (w, h) mm
for ref, (lib_id, val, footprint) in PARTS.items():
    if not footprint:
        continue
    d, name = fp_dir(footprint)
    fp = pcbnew.FootprintLoad(d, name)
    if fp is None:
        print(f"!! could not load {footprint} for {ref}"); continue
    fp.SetReference(ref); fp.SetValue(val)
    cy = fp.GetCourtyard(pcbnew.F_CrtYd).BBox()
    w, h = pcbnew.ToMM(cy.GetWidth()), pcbnew.ToMM(cy.GetHeight())
    if w <= 0 or h <= 0:
        bb = fp.GetBoundingBox(False, False)
        w, h = pcbnew.ToMM(bb.GetWidth()), pcbnew.ToMM(bb.GetHeight())
    FOOT[ref] = fp
    SIZE[ref] = (max(w, 1.0), max(h, 1.0))

# ---- shelf packer: lay refs L->R inside a rectangle, wrapping to new shelves;
# guarantees courtyards do not overlap within a region. ----
PLACE = {}   # ref -> (x, y, rot)
GAP = 1.8
def pack(refs, x0, y0, x1, rotate=None):
    refs = [r for r in refs if r in SIZE]
    cx, cy, shelf_h = x0, y0, 0.0
    for r in refs:
        w, h = SIZE[r]
        rot = (rotate or {}).get(r, 0)
        if rot in (90, 270):
            w, h = h, w
        if cx + w > x1 and cx > x0:        # wrap to next shelf
            cx = x0; cy += shelf_h + GAP; shelf_h = 0.0
        PLACE[r] = (round(cx + w / 2, 2), round(cy + h / 2, 2), rot)
        cx += w + GAP
        shelf_h = max(shelf_h, h)
    return cy + shelf_h                    # bottom edge consumed

# ---- functional regions (each gets its own band of the board) ----
# power-in + PD, top-left
pack(["J1", "U1", "C1", "R1", "R3"],            4, 4, 46)
pack(["CB1", "CB2", "C2"],                       4, 28, 46)
# buck + LDO, top band right of the PD block
pack(["CI1", "U2", "L1", "CO1", "D1"],          48, 4, 119)
pack(["C3", "C4", "U3", "C5", "C6", "C7"],      48, 22, 119)
# DAC, upper-right
pack(["U4", "C8", "C9", "C10", "C11", "C12", "C13", "C14", "R5"], 80, 36, 119)
# user IO (JSTs + pull-ups + LED), mid-left
pack(["J3", "R12", "R13", "R4", "D2"],           4, 44, 46)
pack(["J4", "R14", "R15", "R16", "C20", "C21", "C22"], 4, 54, 46)
# amp input network, centre
pack(["R6", "R7", "C15", "C16", "R17"],         48, 40, 80)
# amp + decoupling + bootstrap, right-centre
pack(["U5", "R8", "R9", "R10", "R11", "C17", "C18", "C19"], 48, 58, 90)
pack(["CP1", "CP2", "CP3", "CBR1", "CBR2", "CBL1", "CBL2"], 48, 74, 90)
# output filter + speaker, far right
pack(["LR1", "LR2", "LL1", "LL2", "CF1", "J2"], 92, 58, 119)
# ESP32 socket placeholders, bottom-left (rotated so the 1x14 strip lies flat)
pack(["U6"], 4, 70, 70, rotate={"U6": 90})
pack(["U7"], 4, 80, 70, rotate={"U7": 90})

# ---- build board ----
try:
    board = pcbnew.CreateEmptyBoard()
except Exception:
    board = pcbnew.BOARD()
try:
    board.GetDesignSettings().m_NetSettings.GetDefaultNetclass().SetClearance(MM(0.15))
except Exception as e:
    print("note: could not set default clearance:", e)

netinfo = {}
for name in sorted(set(pin2net.values())):
    ni = pcbnew.NETINFO_ITEM(board, name); board.Add(ni); netinfo[name] = ni

placed = 0
for ref, fp in FOOT.items():
    cx, cy, rot = PLACE.get(ref, (113, 92, 0))   # packer target = courtyard centre
    fp.SetOrientationDegrees(rot)
    fp.SetPosition(V(cx, cy))
    # footprint origin != courtyard centre (e.g. connectors anchor at pin 1):
    # measure the rotated courtyard centre and shift so it lands on the target.
    bb = fp.GetCourtyard(pcbnew.F_CrtYd).BBox()
    if bb.GetWidth() > 0:
        ccx, ccy = pcbnew.ToMM(bb.GetCenter().x), pcbnew.ToMM(bb.GetCenter().y)
        fp.SetPosition(V(2 * cx - ccx, 2 * cy - ccy))
    board.Add(fp)
    for pad in fp.Pads():
        net = pin2net.get((ref, pad.GetNumber()))
        if net:
            pad.SetNet(netinfo[net])
    placed += 1

# edge cuts
for (x1, y1, x2, y2) in [(0, 0, BOARD_W, 0), (BOARD_W, 0, BOARD_W, BOARD_H),
                         (BOARD_W, BOARD_H, 0, BOARD_H), (0, BOARD_H, 0, 0)]:
    s = pcbnew.PCB_SHAPE(board); s.SetShape(pcbnew.SHAPE_T_SEGMENT)
    s.SetStart(V(x1, y1)); s.SetEnd(V(x2, y2))
    s.SetLayer(pcbnew.Edge_Cuts); s.SetWidth(MM(0.15)); board.Add(s)

# GND pours, both copper layers (saved unfilled; KiCad fills on open)
gnd = netinfo["GND"]
corners = [(0.5, 0.5), (BOARD_W - 0.5, 0.5), (BOARD_W - 0.5, BOARD_H - 0.5), (0.5, BOARD_H - 0.5)]
for layer in (pcbnew.F_Cu, pcbnew.B_Cu):
    z = pcbnew.ZONE(board); z.SetLayer(layer); z.SetNetCode(gnd.GetNetCode())
    z.SetAssignedPriority(0); z.SetLocalClearance(MM(0.3))
    pts = pcbnew.VECTOR_VECTOR2I()
    for (x, y) in corners:
        pts.append(V(x, y))
    z.AddPolygon(pts); board.Add(z)

pcbnew.SaveBoard(os.path.join(HERE, "amp.kicad_pcb"), board)
print(f"wrote amp.kicad_pcb  ({placed} footprints, {len(netinfo)} nets, "
      f"board {BOARD_W:.0f}x{BOARD_H:.0f} mm; zones unfilled - fill on open)")
