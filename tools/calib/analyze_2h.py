#!/usr/bin/env python3
"""
analyze_2h.py — convergence analysis for the long (2 h) capture. The headline
question: does the proportional servo eventually converge — ppm plateau, queue
error settle, acoustic inter-speaker drift flatten — and on what time constant?

The mic WAV is ~1.4 GB, too big for one Hilbert transform, so the acoustic offset
is computed in WINDOWS (read by byte offset, tones.analyze each window → median
offset + within-window drift). Stats are read whole (1 Hz, tiny).

Three panels over the full run: commanded ppm per node, device-queue error
(phaseErr), and the windowed acoustic inter-speaker offset.
"""
from __future__ import annotations
import argparse, json, os
import numpy as np
import tones

SR = tones.SR
BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#2a3340")


def parse_stats(path, pl, ph):
    t0 = None
    t, lo, hi, ph_lo, ph_hi = [], [], [], [], []
    for line in open(path):
        sp = line.strip().split(" ", 1)
        if len(sp) < 2:
            continue
        ts, rest = float(sp[0]), sp[1]
        if rest == "PLAY":
            t0 = ts; continue
        if t0 is None:
            continue
        try:
            arr = json.loads(rest)
        except Exception:
            continue
        d = {n["nodeId"]: n for n in arr}
        a, b = d.get(pl), d.get(ph)
        if a and b and a.get("synced") and b.get("synced"):
            t.append((ts - t0) / 60)
            lo.append(a["ratePPM"]); hi.append(b["ratePPM"])
            ph_lo.append(a.get("phaseErrNs", 0) / 1e6); ph_hi.append(b.get("phaseErrNs", 0) / 1e6)
    return (np.array(t), np.array(lo), np.array(hi), np.array(ph_lo), np.array(ph_hi))


def read_window(path, t0_s, dur_s):
    """Read [t0_s, t0_s+dur_s) from a 44-byte-header s16 stereo WAV by byte offset
    (robust to a not-yet-finalized header — uses file size)."""
    hdr = 44
    fsz = os.path.getsize(path)
    start = hdr + int(t0_s * SR) * 4
    nbytes = int(dur_s * SR) * 4
    if start >= fsz:
        return None
    nbytes = min(nbytes, fsz - start)
    with open(path, "rb") as f:
        f.seek(start)
        raw = f.read(nbytes)
    a = np.frombuffer(raw, dtype="<i2").astype(float).reshape(-1, 2) / 32768
    return a


def acoustic_curve(wav, total_min, win_s=60, step_s=120):
    """Windowed offset: every step_s, analyze a win_s window. Returns t_min, off_us."""
    tc, off = [], []
    t = 0.0
    while t < total_min * 60:
        a = read_window(wav, t, win_s)
        if a is None or len(a) < win_s * SR // 2:
            break
        o, _ = tones.analyze(a)
        if len(o):
            med = np.median(o); mad = np.median(np.abs(o - med)) * 1.4826
            inl = o[np.abs(o - med) < 5 * mad]
            if len(inl):
                tc.append((t + win_s / 2) / 60); off.append(np.median(inl))
        t += step_s
    return np.array(tc), np.array(off)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--wav", default="results/tones_2h.wav")
    ap.add_argument("--stats-log", default="results/tones_stats_2h.jsonl")
    ap.add_argument("--pi-low", required=True)
    ap.add_argument("--pi-high", required=True)
    ap.add_argument("--out", default="results/conv_2h")
    args = ap.parse_args()

    t, lo, hi, phlo, phhi = parse_stats(args.stats_log, args.pi_low, args.pi_high)
    total_min = t.max() if len(t) else 0
    tc, off = acoustic_curve(args.wav, total_min)

    # Convergence summary: compare first vs last 10 min.
    def seg(arr, tt, a, b):
        m = (tt >= a) & (tt < b); return arr[m]
    print(f"run length {total_min:.1f} min | acoustic windows {len(tc)}")
    for nm, ppm in (("pi01", lo), ("pi02", hi)):
        early = np.mean(seg(ppm, t, 1, 6)); late = np.mean(seg(ppm, t, total_min - 10, total_min))
        print(f"  {nm} ppm: first5min {early:+.1f} → last10min {late:+.1f} (settled? Δ={late-early:+.1f})")
    if len(tc) > 4:
        # drift in the last 30 min vs first 10 min (slope of acoustic offset)
        def slope(a, b):
            m = (tc >= a) & (tc < b)
            return np.polyfit(tc[m], off[m], 1)[0] / 60 if m.sum() > 2 else float("nan")
        print(f"  acoustic drift: first10min {slope(0, 10):+.2f} ppm → last30min {slope(total_min-30, total_min):+.2f} ppm")

    import matplotlib; matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    plt.rcParams.update({"figure.facecolor": BG, "axes.facecolor": BG,
        "savefig.facecolor": BG, "text.color": FG, "axes.labelcolor": MUTED,
        "xtick.color": MUTED, "ytick.color": MUTED, "axes.edgecolor": BORDER,
        "axes.grid": True, "grid.color": BORDER, "grid.alpha": 0.5, "font.size": 10})
    fig, (a1, a2, a3) = plt.subplots(3, 1, figsize=(12, 10), dpi=150, sharex=True)
    a1.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    a1.plot(t, lo, color=ACCENT, lw=1.3, label="pi01"); a1.plot(t, hi, color=ACCENT2, lw=1.3, label="pi02")
    a1.set_ylabel("commanded ppm"); a1.legend(loc="best", frameon=False, fontsize=9)
    a1.set_title("Servo convergence over 2 h — does ppm plateau?", color=FG, fontsize=12)
    a2.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    a2.plot(t, phlo, color=ACCENT, lw=1.2, label="pi01 queue err"); a2.plot(t, phhi, color=ACCENT2, lw=1.2, label="pi02 queue err")
    a2.set_ylabel("device-queue err (ms)"); a2.legend(loc="best", frameon=False, fontsize=9)
    if len(tc):
        a3.plot(tc, off - np.median(off[:3]) if len(off) >= 3 else off, color=ACCENT3, lw=1.6, marker="o", ms=3)
    a3.set_ylabel("acoustic offset (µs)"); a3.set_xlabel("time (minutes)")
    a3.set_title("Acoustic inter-speaker offset (windowed) — does drift flatten?", color=FG, fontsize=11)
    fig.subplots_adjust(top=0.95, bottom=0.06, hspace=0.2)
    fig.savefig(args.out + ".svg"); fig.savefig(args.out + ".png")
    print("wrote", args.out + ".svg/.png")
    json.dump({"t_min": t.tolist(), "ppm_lo": lo.tolist(), "ppm_hi": hi.tolist(),
               "acoustic_t": tc.tolist(), "acoustic_us": off.tolist()}, open(args.out + ".json", "w"))


if __name__ == "__main__":
    main()
