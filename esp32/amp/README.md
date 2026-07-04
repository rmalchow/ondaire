# ondaire amp — ESP32-S3 carrier / 50 W class-D node

A KiCad 10 project for a **powered-speaker node**: an ESP32-S3 dev board drops
into a socket, an I²S DAC + class-D amp turn the stream into ~40–50 W of audio,
and the whole thing runs from a single USB-C cable via USB-PD. JST plugs bring
out a rotary encoder and an I²C OLED.

This carrier is the hardware counterpart to the `esp32/` firmware in this repo —
the GPIO map, I²S pins and encoder pins are taken straight from
`esp32/boards/board_esp32s3_*.h`, so a flashed Super Mini or Waveshare Zero
works with no reconfiguration.

> **Status — schematic ERC-clean; board placed + ground-poured, not yet routed.**
> Schematic, symbols, footprints, BOM and netlist are generated and validated
> (`kicad-cli sch erc` → 0 errors). The `.kicad_pcb` has every footprint placed
> in a functional floorplan with GND zones on both layers; DRC is clean except
> the expected unrouted ratsnest (no overlaps/clearance/short errors). **Signal
> and power routing is the remaining manual step.** A few values need a
> datasheet/board check before you order — see
> [Verify before you order](#verify-before-you-order).

---

## Block diagram

```
            ┌───────── USB-C (PD) ─────────┐
   charger  │  VBUS ─┬─────────────► +20V rail ──────────────┐
   ─────────┤  CC1/2─┤ CH224K       (≈3 A @ 20 V)            │
   20V/65W+ │  D±  ───┘ PD sink                              │
            └─────────┘  (CFG1: NC = request 20 V)           │
                                                             │
          +20V ──► LM2596S-5.0 buck ──► +5V ──► AMS1117-3.3 ──► +3V3
                                         │                      │
                                         ▼                      ▼
                                   ESP32-S3 socket        PCM5102A DAC
                                   (Super Mini / Zero)    (MCLK-less I²S)
                                         │  I²S BCK/LRCK/DIN     │
                                         └──────────────────────┘
                                                                │ OUT L/R
                                       L+R passive sum ◄─────────┘
                                                │
                          +20V ──► TPA3116D2 (PBTL mono) ──► LC filter ──► SPEAKER
                                       ▲   ▲                              (4 Ω, ~50 W)
                                  SD/FAULT  gain=20 dB
                                       │
   JST: OLED (I²C SDA/SCL) ────────────┤ ESP32 GPIO
   JST: rotary encoder (A/B/SW) ───────┘
```

## What was decided

| Choice | Decision | Why |
|---|---|---|
| Amp topology | **TPA3116D2, mono PBTL** | One speaker per node; PBTL gives ~40–50 W into 4 Ω from a 20 V rail (min load 1.6 Ω). |
| DAC | **PCM5102A**, MCLK-less | Matches the firmware (`i2s_mclk = -1`, internal PLL); clean 2.1 Vrms line out, no I²C config. |
| Power | **USB-PD 20 V via CH224K** | Single-cable power; 20 V is the TPA3116's sweet spot. Single-resistor select, default = 20 V. |
| ESP socket | **Both Super Mini + Zero** | Populate one footprint; identical nets, so one board fits either dev board. |
| Buck / LDO | LM2596S-5.0 → AMS1117-3.3 | 20 V is above most synchronous bucks' input max; LM2596 (40 V in) is robust. Separate clean 3V3 for the DAC. |

## Signal & power chain (the important bits)

**USB-PD (CH224K).** USB-C VBUS feeds the +20 V rail directly; the CH224K only
negotiates. Voltage is set in *single-resistor* mode: **R1 on CFG1→GND, with
CFG2/CFG3 unconnected.** R1 **unpopulated = 20 V**; 6.8 k = 9 V, 24 k = 12 V,
56 k = 15 V (WCH CH224K DS §5.2.1). D± route to the CH224K for BC1.2 fallback.

**Amp (TPA3116D2 PBTL).** Wired per the TI datasheet mono mode (SLOS708G §7.4.1,
Fig. 36):
- `LINP`, `LINN` → GND (this selects PBTL/mono at power-up).
- Signal into `RINP`/`RINN` (single-ended L+R sum, AC-coupled).
- Outputs paralleled: **`OUTPR ∥ OUTNR` → SPK+**, **`OUTPL ∥ OUTNL` → SPK−**,
  each half-bridge through its own 10 µH inductor, 1 µF across the speaker
  (PBTL filter, min L = 10 µH / C = 1 µF).
- `MODSEL` = GND (BD modulation, lowest THD with the LC filter).
- `PLIMIT` tied to `GVDD` (no power limit). `GAIN/SLV` = 5.6 k to GVDD → **20 dB,
  master mode** (lowest gain the part offers).
- `SDZ` (enable) ← ESP GPIO1 with a 100 k pull-down (amp stays off at boot).
  `FAULTZ` → GPIO4 (10 k pull-up). `MUTE` tied low (software volume handles mute).
- All four `BSxx` bootstrap caps = 220 nF. `AVCC` fed from +20 V through a 3.3 Ω
  RC. PowerPAD → GND (thermal vias).

**Gain / headroom.** At 20 dB the PCM5102A's 2.1 Vrms full-scale over-drives the
20 V rail by a few dB, so the **firmware's digital volume sets the real level** —
keep it below ~−4 dBFS for clean full power (normal listening is well under that).
To hard-limit in hardware, populate **R17** (MIXIN→GND, DNP) to attenuate/divide
the summer.

## GPIO map (matches `esp32/boards/board_esp32s3_*.h`)

| Signal | GPIO | Notes |
|---|---|---|
| I²S BCK | 5 | → PCM5102A BCK |
| I²S LRCK | 6 | → PCM5102A LCK |
| I²S DIN | 7 | → PCM5102A DIN (MCLK-less; SCK→GND) |
| Encoder A / B / SW | 15 / 16 / 17 | KY-040 / EC11 via JST (J4) |
| I²C SDA / SCL | 8 / 18 | SSD1306 OLED via JST (J3) |
| Amp enable (SDZ) | 1 | **new** — drive high to un-shutdown the amp |
| CH224K PG | 2 | **new** — power-good input |
| Amp FAULTZ | 4 | **new** — fault input (open-drain) |

GPIO 1/2/4/8/18 are the carrier-only additions; they're free and conflict-free on
both boards. The firmware doesn't drive amp-enable / fault / OLED yet — the board
is forward-looking on those. (3V3 from the dev board is left unconnected; the
carrier makes its own clean 3V3 for the DAC.)

## Power budget

| Rail | Source | Load | Worst-case |
|---|---|---|---|
| +20 V | USB-PD | TPA3116 PVCC | ~3 A at full output |
| +5 V | LM2596S-5.0 | ESP32 (Wi-Fi TX peaks) + LDO | ~1 A |
| +3V3 | AMS1117-3.3 | DAC + OLED + pull-ups | ~50 mA |

Full output (~50 W audio, ~90 % amp efficiency) + logic ≈ **60 W** → use a
**≥ 65 W (20 V / 3.25 A) charger**; a 100 W (20 V/5 A) supply gives headroom.
A 60 W (20 V/3 A) charger works but limits peak SPL.

## Files

| File | What |
|---|---|
| `amp.kicad_pro` | Project. Open this in KiCad 10. |
| `amp.kicad_sch` | Schematic (generated). |
| `amp.kicad_pcb` | Board: placed + GND-poured, unrouted (generated). |
| `amp.kicad_sym` | Project symbols: TPA3116D2 + the two ESP32 sockets (generated). |
| `amp.pretty/` | Project footprints: ESP32 dev-board sockets (generated, **placeholder geometry**). |
| `sym-lib-table`, `fp-lib-table` | Register the `amp` libs for this project. |
| `bom.csv` | Grouped BOM with LCSC starting points (generated). |
| `gen_*.py`, `ksym.py` | The generators — **edit these, not the outputs.** |

### Regenerating

The schematic/symbols/footprints/BOM are produced from declarative data so the
design is diffable and editable. After changing `gen_sch.py` (netlist/parts):

```sh
cd esp32/amp
python3 gen_symbols.py      # amp.kicad_sym
python3 gen_footprints.py   # amp.pretty/*
python3 gen_sch.py          # amp.kicad_sch
python3 gen_bom.py          # bom.csv
python3 gen_pcb.py          # amp.kicad_pcb (needs KiCad's pcbnew python module)
kicad-cli sch erc amp.kicad_sch   # validate schematic
kicad-cli pcb drc amp.kicad_pcb   # validate board
```

The schematic is a **connectivity-capture** style: components on a grid with a
global label on every pin (same name = same net). It's ERC-clean and exports a
correct netlist; tidy the layout/wiring in Eeschema if you want a pretty sheet.

## PCB layout

`gen_pcb.py` builds the board with the `pcbnew` API: it reads each footprint's
real courtyard size and shelf-packs the parts into functional regions (power-in
+ PD top-left, buck/LDO across the top, DAC upper-right, amp + output filter +
speaker lower-right, sockets + user-IO on the left), then pours **GND on both
copper layers**. The TPA3116 uses the `_ThermalVias` footprint, so its PowerPAD
is already stitched to the back plane.

What's left for you (in Pcbnew):

1. **Fill zones** — they're saved unfilled (a headless-`pcbnew` limitation);
   press `B` or *Edit → Fill All Zones* on open. This connects the whole GND net.
2. **Route** the ~46 non-GND nets. Suggested priorities:
   - Wide tracks for **+20V** and the **speaker outputs** (≥1.5 mm; ~3.5 A) and
     the buck **+5V** path; keep the buck's SW node / catch-diode loop tight.
   - Keep the four **TPA3116 output → inductor → speaker** runs short and
     symmetric; the LC filter and bulk PVCC caps right at the amp.
   - **I²S** (BCK/LRCK/DIN) short from socket to DAC; star the DAC analog ground.
3. **Finalize the ESP32 socket land patterns** (placeholders — see below) and add
   a mechanical support header on the board's far edge.
4. Re-run DRC. The generated board is DRC-clean apart from the unrouted ratsnest
   (no overlap/clearance/short errors) and a couple of cosmetic silk-over-pad
   warnings; 2-layer works, **4-layer is recommended** for a quiet ground plane
   under the amp.

> The placement is a *starting point* — nudge parts to suit your routing. Re-running
> `gen_pcb.py` overwrites `amp.kicad_pcb`, so once you start routing by hand, stop
> regenerating it (keep editing the schematic + re-import instead).

## Assembly (Aisler)

Designed to be SMT-assemblable. All ICs and passives are SMD (0805 jellybean,
HTSSOP/TSSOP/SOT/TO-263). **Hand-soldered (not SMT-placed):** the ESP32 socket
headers, the JST plugs, and the speaker terminal block — Aisler does SMT
assembly, so place these THT parts yourself.

- 2-layer is workable; **prefer 4-layer** for a solid ground plane under the amp.
- Footprints use Aisler/JLC-friendly packages; LCSC codes in `bom.csv` are
  *starting points* — verify against the live catalogue.
- For the TPA3116 PowerPAD, keep the thermal-via footprint variant (it's the one
  assigned) and pour ground top+bottom.

## Verify before you order

These are flagged in the BOM / netlist and need a quick check against your parts:

1. **ESP32 socket footprints** (`amp.pretty/*`) are **placeholders** — 1×14
   headers tapping the GPIO1–18 edge. Super Mini clones vary in length / row
   spacing, and the Zero is castellated+THT. **Measure your actual board** and
   redraw the land pattern (and add a mechanical support header on the far edge).
   Pad numbers already match the symbol, so the netlist survives the edit.
2. **CH224K footprint** — ESSOP-10; confirm the MSOP-10-1EP land matches your
   exact part marking.
3. **Output inductors** — `L_12x12mm_H6mm` placeholder; pick 10 µH parts rated
   **≥ 4 A saturation** (e.g. shielded power inductors), 33 µH ≥ 3 A for the buck.
4. **Electrolytics** — `CP_Elec_8x10.5` placeholder; match diameter/pitch to the
   470 µF/35 V and 220–330 µF parts you actually buy.
5. **Amp gain** (R9) and **AM switching-frequency** pins (AM0/1/2 → GND default)
   per the TPA3116 datasheet if you want something other than 20 dB / default fsw.

## References

- TI TPA3116D2 datasheet (SLOS708G) — pinout, PBTL mono (§7.4.1), filters.
- WCH CH224K datasheet — single-resistor voltage select (§5.2.1).
- TI PCM5102A datasheet — MCLK-less / internal-PLL operation.
- `esp32/boards/board_esp32s3_*.h` — the firmware's GPIO truth.
