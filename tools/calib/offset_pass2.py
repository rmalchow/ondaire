#!/usr/bin/env python3
"""
offset_pass2.py — second pass on the acoustic inter-speaker offset (R-L) with
proper outlier rejection + percentile views.

Two things matter here:
  * The offset is a SAWTOOTH (servo wind-up/reset), not a stationary line, so a
    global median/MAD reject would throw away the real reset peaks. We reject
    against a ROLLING median instead (lr_drift.py style): a point is an outlier
    only if it's far from its LOCAL neighbourhood — that kills isolated mispaired
    / reverb-mis-picked edges while keeping the ramps and resets.
  * Percentiles describe the spread the cleaned signal actually occupies — both
    the overall CDF and how the distribution drifts over time (rolling bands).

Dense, full-coverage pass: contiguous 60 s windows (no 120 s skip), tones.analyze
each, concatenate every per-cycle offset. Memory-safe windowed byte-offset reads.

Outputs:
  <out>_clean.*  — raw offset with rejected outliers marked + cleaned trace
  <out>_pct.*    — CDF of the cleaned offset + rolling percentile bands over time
  <out>_pass2.json
"""
from __future__ import annotations
import argparse, json, os
import numpy as np
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import tones

SR = 48_000
HDR = 44
BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#2a3340")
REJECT = "#ff5c7a"


def style(ax):
    ax.set_facecolor(BG)
    for s in ax.spines.values():
        s.set_color(BORDER)
    ax.tick_params(colors=MUTED, labelsize=8)
    ax.grid(True, color=BORDER, lw=0.5, alpha=0.5)
    ax.xaxis.label.set_color(MUTED); ax.yaxis.label.set_color(MUTED)


def save(f, out, name):
    f.tight_layout()
    f.savefig(f"{out}_{name}.png", dpi=120, facecolor=BG)
    f.savefig(f"{out}_{name}.svg", facecolor=BG)
    plt.close(f)
    print(f"wrote {out}_{name}.svg/.png")


def read_window(path, t0_s, dur_s):
    fsz = os.path.getsize(path)
    start = HDR + int(t0_s * SR) * 4
    if start >= fsz:
        return None
    nbytes = min(int(dur_s * SR) * 4, fsz - start)
    with open(path, "rb") as fp:
        fp.seek(start); raw = fp.read(nbytes)
    return np.frombuffer(raw, "<i2").astype(float).reshape(-1, 2) / 32768


def dense_offsets(path, win_s=60):
    """Every per-cycle R-L offset over the whole file (contiguous windows)."""
    fsz = os.path.getsize(path)
    total_s = (fsz - HDR) / 4 / SR
    tc, off = [], []
    t = 0.0
    while t < total_s:
        a = read_window(path, t, win_s)
        if a is None or len(a) < win_s * SR // 2:
            break
        o, tm = tones.analyze(a)
        for k in range(len(o)):
            tc.append(t / 60 + tm[k]); off.append(o[k])
        t += win_s
    tc, off = np.array(tc), np.array(off)
    # dedupe near-identical timestamps at window seams
    if len(tc):
        keep = np.concatenate([[True], np.diff(tc) > 1e-6])
        tc, off = tc[keep], off[keep]
    return tc, off


def rolling_median(y, w):
    """Centered rolling median, edge-padded by reflection."""
    n = len(y); h = w // 2
    pad = np.pad(y, h, mode="reflect")
    out = np.empty(n)
    for i in range(n):
        out[i] = np.median(pad[i:i + w])
    return out


def reject_rolling(t, y, w=21, k=4.0):
    """Outlier = far from the LOCAL rolling median (in rolling-MAD units).
    Preserves slow ramps and sharp resets; removes isolated spikes."""
    med = rolling_median(y, w)
    resid = y - med
    mad = np.median(np.abs(resid - np.median(resid))) * 1.4826 or 1.0
    keep = np.abs(resid) < k * mad
    return keep, med, mad


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--wav", required=True)
    ap.add_argument("--out", default="results/offset_pass2")
    ap.add_argument("--label", default="")
    ap.add_argument("--win-cycles", type=int, default=21, help="rolling-median window (cycles)")
    ap.add_argument("--k", type=float, default=4.0, help="reject threshold (rolling-MAD units)")
    args = ap.parse_args()

    tc, off = dense_offsets(args.wav)
    if len(off) < 20:
        print("too few cycles"); return
    keep, med, mad = reject_rolling(tc, off, args.win_cycles, args.k)
    tcl, ocl = tc[keep], off[keep]
    nrej = int((~keep).sum())
    print(f"cycles {len(off)} | rejected {nrej} ({100*nrej/len(off):.1f}%) | rolling-MAD {mad:.0f} µs")

    # percentile table (cleaned)
    ps = [1, 5, 10, 25, 50, 75, 90, 95, 99]
    pv = {f"p{p}": float(np.percentile(ocl, p)) for p in ps}
    iqr = pv["p75"] - pv["p25"]
    span_90 = pv["p95"] - pv["p5"]
    summary = {
        "cycles": int(len(off)), "rejected": nrej, "reject_pct": round(100 * nrej / len(off), 2),
        "rolling_mad_us": float(mad), "win_cycles": args.win_cycles, "k": args.k,
        "percentiles_us": pv, "iqr_us": float(iqr), "p5_p95_span_us": float(span_90),
        "min_us": float(ocl.min()), "max_us": float(ocl.max()), "mean_us": float(ocl.mean()),
    }
    print("percentiles (µs):", json.dumps(pv, indent=2))
    print(f"IQR {iqr:.0f} µs | p5–p95 span {span_90:.0f} µs")

    # ---------- figure 1: cleaned trace ----------
    # Y-zoom to the cleaned signal (+ headroom) so the wild ±100-200 ms mispaired
    # edges don't squash the view; they're clipped but counted in the legend.
    lo_y, hi_y = np.percentile(ocl, 0.5) / 1000, np.percentile(ocl, 99.5) / 1000
    pad = max(2.0, 0.15 * (hi_y - lo_y))
    f, ax = plt.subplots(figsize=(11, 5)); f.patch.set_facecolor(BG)
    if nrej:
        ax.scatter(tc[~keep], off[~keep] / 1000, color=REJECT, s=10, zorder=2,
                   label=f"rejected ({nrej}, {100*nrej/len(off):.1f}%)")
    ax.plot(tc, med / 1000, color=ACCENT3, lw=2.2, alpha=0.55, zorder=3, label="rolling median")
    ax.plot(tcl, ocl / 1000, color=ACCENT, lw=0.7, zorder=4, label="cleaned offset")
    ax.set_ylim(lo_y - pad, hi_y + pad)
    ax.legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8, loc="upper right")
    ax.set_title(f"Inter-speaker offset R-L — rolling-median outlier rejection {args.label}",
                 color=FG, fontsize=11)
    ax.set_ylabel("offset (ms)"); ax.set_xlabel("time (min)")
    style(ax)
    save(f, args.out, "clean")

    # ---------- figure 2: percentiles ----------
    f = plt.figure(figsize=(11, 5)); f.patch.set_facecolor(BG)
    a0 = f.add_subplot(1, 2, 1)   # CDF
    a1 = f.add_subplot(1, 2, 2)   # rolling percentile bands

    xs = np.sort(ocl) / 1000
    cdf = np.linspace(0, 100, len(xs))
    a0.plot(xs, cdf, color=ACCENT, lw=1.4)
    for p, c, lbl in [(5, ACCENT2, "p5"), (50, ACCENT3, "p50"), (95, ACCENT2, "p95")]:
        v = pv[f"p{p}"] / 1000
        a0.axvline(v, color=c, ls="--", lw=0.9, alpha=0.8)
        a0.text(v, 3 + (p == 95) * 8, f" {lbl} {v:.1f}ms", color=c, fontsize=7, rotation=90, va="bottom")
    a0.set_title("CDF of cleaned offset", color=FG, fontsize=10)
    a0.set_xlabel("offset (ms)"); a0.set_ylabel("percentile (%)")

    # rolling percentile bands over time (windows of W cycles)
    W = max(15, len(ocl) // 80)
    tb, pb = [], {p: [] for p in (5, 25, 50, 75, 95)}
    for i in range(0, len(ocl) - W, max(1, W // 2)):
        seg = ocl[i:i + W]
        tb.append(np.median(tcl[i:i + W]))
        for p in pb:
            pb[p].append(np.percentile(seg, p))
    tb = np.array(tb)
    a1.fill_between(tb, np.array(pb[5]) / 1000, np.array(pb[95]) / 1000, color=ACCENT, alpha=0.15, label="p5–p95")
    a1.fill_between(tb, np.array(pb[25]) / 1000, np.array(pb[75]) / 1000, color=ACCENT, alpha=0.30, label="p25–p75")
    a1.plot(tb, np.array(pb[50]) / 1000, color=ACCENT3, lw=1.2, label="median")
    a1.legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8, loc="upper right")
    a1.set_title("Rolling percentile bands over time", color=FG, fontsize=10)
    a1.set_xlabel("time (min)"); a1.set_ylabel("offset (ms)")
    for a in (a0, a1):
        style(a)
    save(f, args.out, "pct")

    json.dump(summary, open(f"{args.out}_pass2.json", "w"), indent=2)


if __name__ == "__main__":
    main()
