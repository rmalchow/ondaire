#!/usr/bin/env python3
"""
hero.py — marketing "closeness" graphs for a dual-tone capture.

"Closeness" = how tightly the two speakers hold their RELATIVE alignment: the
acoustic inter-speaker offset (R−L) with the static physical/device baseline removed
(de-meaned), so what's left is the sync precision. We omit the startup settle
(`--skip-min`), reject mispaired-edge measurement spikes (rolling median, as in
offset_pass2), and report percentiles of |deviation| — e.g. "99.5% within X ms".

Two figures:
  <out>_time.* — closeness over time with ±p50/±p95/±p99 bands (the flat line)
  <out>_cdf.*  — the closeness CDF with the headline percentile callouts

Note: the acoustic measurement carries its own ~100–300 µs mic/room noise, so these
percentiles are an UPPER BOUND — the true electrical sync is at least this tight.
"""
from __future__ import annotations
import argparse, json, os
import numpy as np
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import tones
import offset_pass2 as op  # read_window(), reject_rolling()


def single_pass_offsets(path, skip_min):
    """Analyze the whole post-startup span in ONE tones.analyze call — no 60 s window
    seams (which shift the per-window baseline by a fraction of a ms). Returns
    (t_min_absolute, offset_us)."""
    total_s = (os.path.getsize(path) - op.HDR) / 4 / op.SR
    a = op.read_window(path, skip_min * 60, total_s - skip_min * 60)
    off, tm = tones.analyze(a)            # tm is minutes from the window start
    return tm + skip_min, off


def smooth_median(y, w):
    """Centered rolling median for the display line (reflect-padded)."""
    if w < 2:
        return y
    h = w // 2
    pad = np.pad(y, h, mode="reflect")
    return np.array([np.median(pad[i:i + w]) for i in range(len(y))])

BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#2a3340")


def style(ax):
    ax.set_facecolor(BG)
    for s in ax.spines.values():
        s.set_color(BORDER)
    ax.tick_params(colors=MUTED, labelsize=9)
    ax.grid(True, color=BORDER, lw=0.5, alpha=0.5)
    ax.xaxis.label.set_color(MUTED)
    ax.yaxis.label.set_color(MUTED)


def save(f, out, name):
    f.tight_layout()
    f.savefig(f"{out}_{name}.png", dpi=140, facecolor=BG)
    f.savefig(f"{out}_{name}.svg", facecolor=BG)
    plt.close(f)
    print(f"wrote {out}_{name}.svg/.png")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--wav", required=True)
    ap.add_argument("--out", default="results/hero")
    ap.add_argument("--skip-min", type=float, default=2.5, help="omit the startup settle")
    ap.add_argument("--label", default="")
    ap.add_argument("--detrend", action="store_true", help="also remove the slow linear drift")
    ap.add_argument("--bare", action="store_true",
                    help="marketing mode: no title, no headline text, no time-axis numbers "
                         "(the page overlays its own brand text)")
    args = ap.parse_args()

    tc, off = single_pass_offsets(args.wav, args.skip_min)
    keep, _, _ = op.reject_rolling(tc, off, w=21, k=4.0)
    tcl, ocl = tc[keep], off[keep]
    nrej = int((~keep).sum())

    center = np.median(ocl)
    clos = ocl - center
    if args.detrend:
        sl, ic = np.polyfit(tcl, clos, 1)
        clos = clos - (sl * tcl + ic)
    aclos = np.abs(clos)

    pcts = [50, 90, 95, 99, 99.5, 99.9]
    pv = {p: float(np.percentile(aclos, p)) for p in pcts}
    summary = {
        "skip_min": args.skip_min, "cycles_used": int(len(ocl)), "rejected": nrej,
        "window_min": [float(tcl.min()), float(tcl.max())],
        "baseline_us": float(center), "detrended": args.detrend,
        "closeness_abs_percentiles_us": pv, "rms_us": float(np.sqrt(np.mean(clos ** 2))),
        "max_abs_us": float(aclos.max()),
    }
    print(json.dumps(summary, indent=2))
    # headline: tightest round threshold that p99.5 clears
    p995 = pv[99.5]
    print(f"\nHEADLINE: 99.5% within {p995/1000:.2f} ms  |  99% within {pv[99]/1000:.2f} ms  |  "
          f"median {pv[50]:.0f} µs")

    # ---------- figure 1: closeness over time ----------
    f, ax = plt.subplots(figsize=(12, 5.2))
    f.patch.set_facecolor(BG)
    cm = clos / 1000.0
    for p, c, a in [(99, ACCENT3, 0.10), (95, ACCENT2, 0.14), (50, ACCENT, 0.20)]:
        v = pv[p] / 1000.0
        ax.axhspan(-v, v, color=c, alpha=a, lw=0,
                   label=f"±p{p} = ±{v*1000:.0f} µs" if p != 99.5 else None)
    ax.plot(tcl, cm, color=ACCENT, lw=0.5, alpha=0.30)                 # raw per-cycle
    ax.plot(tcl, smooth_median(clos, 15) / 1000.0, color=ACCENT, lw=1.8)  # ~12 s smoothed
    ax.axhline(0, color=MUTED, lw=0.8, ls=":")
    ax.set_ylim(-1.2, 1.2)
    ax.legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=9, loc="upper right", ncol=3)
    ax.set_ylabel("deviation from alignment (ms)")
    if args.bare:
        ax.set_xticks([])                 # remove time-axis numbering
        ax.set_xlabel("time →")
    else:
        ax.set_xlabel("time (min)")
        ax.set_title(f"Inter-speaker sync — closeness over time {args.label}", color=FG, fontsize=14, pad=10)
        ax.text(0.012, 0.04, f"99.5% within {p995/1000:.2f} ms   ·   99% within {pv[99]/1000:.2f} ms"
                f"   ·   median {pv[50]:.0f} µs",
                transform=ax.transAxes, color=ACCENT, fontsize=11, fontweight="bold",
                va="bottom", ha="left")
    style(ax)
    save(f, args.out, "time")

    # ---------- figure 2: closeness CDF ----------
    f, ax = plt.subplots(figsize=(9, 6))
    f.patch.set_facecolor(BG)
    xs = np.sort(aclos) / 1000.0
    cdf = np.linspace(0, 100, len(xs))
    ax.set_ylim(40, 100.5)
    ax.plot(xs, cdf, color=ACCENT, lw=2.2)
    ax.fill_between(xs, 40, cdf, color=ACCENT, alpha=0.10)
    ax.axvline(1.0, color=ACCENT3, ls="--", lw=1.2, alpha=0.8)
    ax.text(1.0, 41.5, " 1 ms target", color=ACCENT3, fontsize=9, rotation=90, va="bottom")
    # annotate a few, well-spaced percentiles (label offset dodges to avoid collisions)
    for p, dy in [(50, -4), (90, -4), (99.5, 8)]:
        v = pv[p] / 1000.0
        ax.plot([v, v], [40, p], color=MUTED, ls=":", lw=0.8)
        ax.plot([0, v], [p, p], color=MUTED, ls=":", lw=0.8)
        ax.scatter([v], [p], color=ACCENT2, s=28, zorder=5)
        ax.annotate(f"p{p} = {v*1000:.0f} µs", (v, p), textcoords="offset points",
                    xytext=(10, dy), color=FG, fontsize=9.5, fontweight="bold")
    ax.set_xlim(0, max(1.25, xs.max() * 1.02))
    ax.set_xlabel("|deviation from alignment| (ms)")
    ax.set_ylabel("percentile of cycles (%)")
    if not args.bare:
        ax.set_title(f"Inter-speaker sync — closeness CDF {args.label}", color=FG, fontsize=14, pad=10)
    style(ax)
    save(f, args.out, "cdf")

    json.dump(summary, open(f"{args.out}.json", "w"), indent=2)


if __name__ == "__main__":
    main()
