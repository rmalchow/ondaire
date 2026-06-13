#!/usr/bin/env python3
"""
graph_split.py — plot two consecutive captures (a full session restart between
them) on one timeline at the same scale, to test thermal vs servo-convergence.

Top: commanded ppm per node, run1 then run2 (run2 shifted to follow run1 with a
small gap). The servo resets on restart, so run2 starts at ~0 regardless; the
question is whether it ramps to the SAME level as run1 (static crystal offset, the
ramp is just the slow proportional servo re-converging) or HIGHER (physical drift
accumulated during the gap → thermal).

Bottom: acoustic inter-speaker offset, each run re-zeroed to its own start so the
DRIFT RATE (slope) is comparable. Same conclusion axis: equal slope → static;
steeper run2 → thermal.
"""
from __future__ import annotations
import argparse, json
import numpy as np
import tones

BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#2a3340")
GAP = 0.1  # minutes of visual gap drawn between the two runs


def parse_stats(path, pl, ph):
    t0 = None
    t, lo, hi = [], [], []
    for line in open(path):
        sp = line.strip().split(" ", 1)
        if len(sp) < 2:
            continue
        ts, rest = float(sp[0]), sp[1]
        if rest == "PLAY":
            t0 = ts
            continue
        if t0 is None:
            continue
        try:
            arr = json.loads(rest)
        except Exception:
            continue
        d = {n["nodeId"]: n for n in arr}
        a, b = d.get(pl), d.get(ph)
        if a and b and a.get("synced") and b.get("synced"):
            t.append((ts - t0) / 60); lo.append(a["ratePPM"]); hi.append(b["ratePPM"])
    return np.array(t), np.array(lo), np.array(hi)


def acoustic(wav, skip=0.1):
    off, tm = tones.analyze(tones.read_wav_stereo(wav))
    med = np.median(off); mad = np.median(np.abs(off - med)) * 1.4826
    k = (np.abs(off - med) < 5 * mad) & (tm >= skip)
    off, tm = off[k], tm[k]
    off = off - np.median(off[tm < tm.min() + 0.4])  # re-zero to this run's start
    return tm, off


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pi-low", required=True)
    ap.add_argument("--pi-high", required=True)
    ap.add_argument("--run1", default="split1")
    ap.add_argument("--run2", default="split2")
    ap.add_argument("--out", default="results/split")
    args = ap.parse_args()
    R = "results/tones_"

    t1, lo1, hi1 = parse_stats(f"{R}stats_{args.run1}.jsonl", args.pi_low, args.pi_high)
    t2, lo2, hi2 = parse_stats(f"{R}stats_{args.run2}.jsonl", args.pi_low, args.pi_high)
    span = t1.max()  # ~3 min; run2 is drawn starting here + GAP
    off2start = span + GAP

    am1, ao1 = acoustic(f"{R}{args.run1}.wav")
    am2, ao2 = acoustic(f"{R}{args.run2}.wav")

    def slope(t, y):
        return np.polyfit(t, y, 1)[0]  # per minute
    print(f"run1: ppm pi01 {lo1[0]:+.1f}→{lo1[-1]:+.1f}  pi02 {hi1[0]:+.1f}→{hi1[-1]:+.1f}"
          f" | acoustic drift {slope(am1, ao1):+.0f} us/min ({slope(am1, ao1)/60:+.2f} ppm)")
    print(f"run2: ppm pi01 {lo2[0]:+.1f}→{lo2[-1]:+.1f}  pi02 {hi2[0]:+.1f}→{hi2[-1]:+.1f}"
          f" | acoustic drift {slope(am2, ao2):+.0f} us/min ({slope(am2, ao2)/60:+.2f} ppm)")
    print(f"pi01 ramp slope: run1 {slope(t1, lo1):+.1f} ppm/min  run2 {slope(t2, lo2):+.1f} ppm/min")
    print(f"VERDICT hint: run2 end ≈ run1 end ⇒ static offset + slow servo; run2 ≫ run1 ⇒ thermal")

    import matplotlib; matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    plt.rcParams.update({"figure.facecolor": BG, "axes.facecolor": BG,
        "savefig.facecolor": BG, "text.color": FG, "axes.labelcolor": MUTED,
        "xtick.color": MUTED, "ytick.color": MUTED, "axes.edgecolor": BORDER,
        "axes.grid": True, "grid.color": BORDER, "grid.alpha": 0.5, "font.size": 10})
    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(11, 8), dpi=160)

    for ax, title, yl in ((ax1, "Commanded ppm — run 1, [restart], run 2", "commanded rate (ppm)"),
                          (ax2, "Acoustic inter-speaker offset (re-zeroed per run)", "offset R−L (µs)")):
        ax.axvline(span + GAP / 2, color=ACCENT3, lw=1.2, ls="--", alpha=0.8)
        ax.text(span + GAP / 2, 0.98, " full restart", color=ACCENT3, fontsize=8,
                ha="left", va="top", transform=ax.get_xaxis_transform())
        ax.set_title(title, color=FG, fontsize=12); ax.set_ylabel(yl)

    ax1.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    ax1.plot(t1, lo1, color=ACCENT, lw=1.6, label="pi01")
    ax1.plot(t1, hi1, color=ACCENT2, lw=1.6, label="pi02")
    ax1.plot(t2 + off2start, lo2, color=ACCENT, lw=1.6)
    ax1.plot(t2 + off2start, hi2, color=ACCENT2, lw=1.6)
    ax1.legend(loc="upper left", frameon=False, fontsize=9)

    ax2.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    ax2.plot(am1, ao1, color=ACCENT3, lw=1.4)
    ax2.plot(am2 + off2start, ao2, color=ACCENT3, lw=1.4)
    ax2.set_xlabel("time (minutes) — run 1 | run 2 consecutive")

    fig.subplots_adjust(top=0.94, bottom=0.07, hspace=0.22)
    fig.savefig(args.out + ".svg"); fig.savefig(args.out + ".png")
    print("wrote", args.out + ".svg/.png")


if __name__ == "__main__":
    main()
