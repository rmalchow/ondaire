#!/usr/bin/env python3
"""
extras.py — supplementary graphs for a dual-tone capture, beyond the canned
analyze_servo / graph_servo_ppm / analyze_2h tools. Three figures:

  1. <out>_glitch.*    — dropout hunt. Per-burst peak energy + gate-cycle period
     straight off the recorded WAV. The played tones are gated 0.40 s on / 0.40 s
     off (0.80 s period); a REAL playout dropout shows up as a burst with
     near-zero peak energy or a gate interval that jumps to a multiple of 0.80 s
     (a missing burst). This is the graph that tells gating apart from glitching.
  2. <out>_jitter.*    — inter-speaker (R-L) offset stability: the full per-cycle
     offset series (de-spiked) with its trend, a jitter histogram (RMS/p95 of the
     detrended residual), and the Allan deviation vs averaging time τ.
  3. <out>_actuator.*  — servo actuator activity from the stats log: cumulative
     samplesDropped/Injected per node and the per-interval drop rate, showing how
     hard the resampler is working and whether it settles.

Memory-safe: the WAV is read in windows by byte offset (handles the 1 h / 2 h
captures). Reuses tones.py DSP and the house brand styling.
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
GATE_PERIOD = tones.ON_S + tones.OFF_S   # 0.80 s
BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#2a3340")


def style(ax):
    ax.set_facecolor(BG)
    for s in ax.spines.values():
        s.set_color(BORDER)
    ax.tick_params(colors=MUTED, labelsize=8)
    ax.grid(True, color=BORDER, lw=0.5, alpha=0.5)
    ax.xaxis.label.set_color(MUTED); ax.yaxis.label.set_color(MUTED)


def fig(n=1, h=6.5):
    f, ax = plt.subplots(n, 1, figsize=(11, h)) if n > 1 else plt.subplots(figsize=(11, h))
    f.patch.set_facecolor(BG)
    return f, (ax if n > 1 else [ax])


def save(f, out, name):
    f.tight_layout()
    f.savefig(f"{out}_{name}.png", dpi=120, facecolor=BG)
    f.savefig(f"{out}_{name}.svg", facecolor=BG)
    plt.close(f)
    print(f"wrote {out}_{name}.svg/.png")


# ---- WAV frame energy (streamed in chunks; no full-file float array) ----------
def frame_rms(path, hop_s=0.005):
    """Per-frame broadband RMS of the (mono-folded) recording, hop_s frames."""
    fsz = os.path.getsize(path)
    nsamp = (fsz - HDR) // 4
    hop = int(hop_s * SR)
    nfr = nsamp // hop
    rms = np.empty(nfr, np.float32)
    chunk_fr = 4096                       # frames per read (~84 MB float at most)
    with open(path, "rb") as fp:
        fp.seek(HDR)
        k = 0
        while k < nfr:
            m = min(chunk_fr, nfr - k)
            raw = fp.read(m * hop * 4)
            if len(raw) < m * hop * 4:
                m = len(raw) // (hop * 4)
                raw = raw[: m * hop * 4]
            if m == 0:
                break
            a = np.frombuffer(raw, "<i2").astype(np.float32).reshape(m, hop, 2)
            mono = a.mean(2) / 32768.0
            rms[k:k + m] = np.sqrt((mono ** 2).mean(1))
            k += m
    t = np.arange(len(rms[:k])) * hop_s
    return t, rms[:k]


def bursts(t, rms):
    """Onset times + per-burst peak energy from the gated envelope."""
    hi = np.percentile(rms, 90)
    thr = 0.3 * hi
    above = rms > thr
    onset = np.where((~above[:-1]) & (above[1:]))[0] + 1
    # one onset per gate cycle: enforce min gap of 0.5 s
    keep, last = [], -1e9
    for i in onset:
        if t[i] - last >= 0.5:
            keep.append(i); last = t[i]
    keep = np.array(keep)
    peaks, ot = [], []
    for j, i in enumerate(keep):
        i2 = keep[j + 1] if j + 1 < len(keep) else len(rms)
        peaks.append(rms[i:i2].max() if i2 > i else rms[i])
        ot.append(t[i])
    return np.array(ot), np.array(peaks)


# ---- windowed per-cycle inter-speaker offsets (whole run) ---------------------
def read_window(path, t0_s, dur_s):
    fsz = os.path.getsize(path)
    start = HDR + int(t0_s * SR) * 4
    if start >= fsz:
        return None
    nbytes = min(int(dur_s * SR) * 4, fsz - start)
    with open(path, "rb") as f:
        f.seek(start); raw = f.read(nbytes)
    return np.frombuffer(raw, "<i2").astype(float).reshape(-1, 2) / 32768


def offsets(path, win_s=60, step_s=120):
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
        t += step_s
    return np.array(tc), np.array(off)


def allan(x, taus_n):
    """Overlapping Allan deviation of series x (uniform samples) for the given
    bin counts n (samples averaged per cluster)."""
    devs = []
    for n in taus_n:
        if n < 1 or 2 * n >= len(x):
            devs.append(np.nan); continue
        # cluster means
        m = len(x) // n
        cl = x[: m * n].reshape(m, n).mean(1)
        d = np.diff(cl)
        devs.append(np.sqrt(0.5 * np.mean(d ** 2)))
    return np.array(devs)


def despike(t, y, k=5.0):
    med = np.median(y); mad = np.median(np.abs(y - med)) * 1.4826 or 1.0
    m = np.abs(y - med) < k * mad
    return t[m], y[m]


# ---- stats (actuator) --------------------------------------------------------
def parse_stats(path, lo_id, hi_id):
    t0 = None
    rows = {k: [] for k in ("t", "inj_lo", "inj_hi", "drop_lo", "drop_hi")}
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
        a, b = d.get(lo_id), d.get(hi_id)
        if not a or not b or not (a.get("synced") and b.get("synced")):
            continue
        rows["t"].append((ts - t0) / 60)
        rows["inj_lo"].append(a["samplesInjected"]); rows["inj_hi"].append(b["samplesInjected"])
        rows["drop_lo"].append(a["samplesDropped"]); rows["drop_hi"].append(b["samplesDropped"])
    return {k: np.array(v, float) for k, v in rows.items()}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--wav", required=True)
    ap.add_argument("--stats-log", required=True)
    ap.add_argument("--pi-low", required=True)
    ap.add_argument("--pi-high", required=True)
    ap.add_argument("--out", default="results/extras")
    ap.add_argument("--label", default="")
    args = ap.parse_args()
    summary = {}

    # ---------- 1. glitch hunt ----------
    t, rms = frame_rms(args.wav)
    ot, peaks = bursts(t, rms)
    iod = np.diff(ot)                       # inter-onset intervals
    pkmed = np.median(peaks)
    weak = peaks < 0.3 * pkmed              # near-silent bursts
    longgap = iod > 1.5 * GATE_PERIOD       # a missing burst
    n_missing = int(np.round(np.sum(np.clip(iod / GATE_PERIOD - 1, 0, None))))
    summary["glitch"] = {
        "bursts": int(len(ot)), "weak_bursts": int(weak.sum()),
        "long_gaps": int(longgap.sum()), "est_missing_bursts": n_missing,
        "gate_period_s_median": float(np.median(iod)) if len(iod) else None,
        "gate_period_s_expected": GATE_PERIOD,
    }
    f, ax = fig(2)
    ax[0].plot(ot / 60, peaks, color=ACCENT, lw=0.8)
    if weak.any():
        ax[0].scatter(ot[weak] / 60, peaks[weak], color="#ff5c7a", s=18, zorder=5, label="weak burst")
        ax[0].legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8)
    ax[0].axhline(0.3 * pkmed, color=MUTED, ls=":", lw=0.8)
    ax[0].set_title(f"Dropout hunt — per-burst peak energy {args.label}", color=FG, fontsize=11)
    ax[0].set_ylabel("burst peak RMS")
    ax[1].plot(ot[1:] / 60, iod, color=ACCENT2, lw=0.6)
    ax[1].axhline(GATE_PERIOD, color=MUTED, ls="--", lw=0.8, label=f"expected {GATE_PERIOD:.2f}s")
    if longgap.any():
        ax[1].scatter(ot[1:][longgap] / 60, iod[longgap], color="#ff5c7a", s=18, zorder=5, label="missing burst")
    ax[1].legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8)
    ax[1].set_title("Gate-cycle interval (steady = no dropouts)", color=FG, fontsize=11)
    ax[1].set_ylabel("interval (s)"); ax[1].set_xlabel("time (min)")
    for a in ax:
        style(a)
    save(f, args.out, "glitch")

    # ---------- 2. jitter & Allan ----------
    tc, off = offsets(args.wav)
    if len(off) > 8:
        tci, offi = despike(tc, off)
        sl, ic = np.polyfit(tci, offi, 1)         # µs per min
        resid = offi - (sl * tci + ic)
        rms_us = float(np.sqrt(np.mean(resid ** 2)))
        p95 = float(np.percentile(np.abs(resid), 95))
        drift_ppm = float(sl / 60.0)
        # Allan on a uniform grid (median dt)
        dt = np.median(np.diff(tci)) * 60 if len(tci) > 1 else 1.0
        taus_n = np.unique(np.round(np.logspace(0, np.log10(max(2, len(offi) // 4)), 18)).astype(int))
        adev = allan(offi - (sl * tci + ic), taus_n)
        taus_s = taus_n * dt
        summary["jitter"] = {"rms_us": rms_us, "p95_us": p95, "drift_ppm": drift_ppm,
                             "cycles": int(len(off)), "baseline_us": float(np.median(off))}
        f = plt.figure(figsize=(11, 7)); f.patch.set_facecolor(BG)
        a0 = f.add_subplot(2, 1, 1)
        a1 = f.add_subplot(2, 2, 3)
        a2 = f.add_subplot(2, 2, 4)
        a0.plot(tci, offi, color=ACCENT, lw=0.5, alpha=0.8)
        a0.plot(tci, sl * tci + ic, color=ACCENT3, lw=1.2, label=f"trend {sl:+.1f} µs/min = {drift_ppm:+.2f} ppm")
        a0.legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8)
        a0.set_title(f"Inter-speaker offset R-L (de-spiked) {args.label}", color=FG, fontsize=11)
        a0.set_ylabel("offset (µs)"); a0.set_xlabel("time (min)")
        a1.hist(resid, bins=40, color=ACCENT2, alpha=0.85)
        a1.set_title(f"Detrended jitter — RMS {rms_us:.0f} µs · p95 {p95:.0f} µs", color=FG, fontsize=10)
        a1.set_xlabel("residual (µs)"); a1.set_ylabel("count")
        good = np.isfinite(adev)
        a2.loglog(taus_s[good], adev[good], "o-", color=ACCENT, ms=4, lw=1)
        a2.set_title("Allan deviation (stability vs τ)", color=FG, fontsize=10)
        a2.set_xlabel("averaging time τ (s)"); a2.set_ylabel("σ (µs)")
        for a in (a0, a1, a2):
            style(a)
        save(f, args.out, "jitter")
    else:
        print("jitter: too few offset cycles")

    # ---------- 3. actuator ----------
    s = parse_stats(args.stats_log, args.pi_low, args.pi_high)
    if len(s["t"]) > 4:
        d_lo = s["drop_lo"] - s["drop_lo"][0]
        d_hi = s["drop_hi"] - s["drop_hi"][0]
        i_lo = s["inj_lo"] - s["inj_lo"][0]
        i_hi = s["inj_hi"] - s["inj_hi"][0]
        dt_s = np.diff(s["t"] * 60, prepend=s["t"][0] * 60)
        dt_s[dt_s <= 0] = 1.0
        rate_lo = np.diff(d_lo, prepend=0) / dt_s
        rate_hi = np.diff(d_hi, prepend=0) / dt_s
        summary["actuator"] = {
            "final_drop_lo": float(d_lo[-1]), "final_drop_hi": float(d_hi[-1]),
            "final_inj_lo": float(i_lo[-1]), "final_inj_hi": float(i_hi[-1]),
            "drop_rate_lo_persec": float(np.mean(rate_lo)), "drop_rate_hi_persec": float(np.mean(rate_hi)),
        }
        f, ax = fig(2)
        ax[0].plot(s["t"], d_lo, color=ACCENT, lw=1, label="pi01 dropped")
        ax[0].plot(s["t"], d_hi, color=ACCENT2, lw=1, label="pi02 dropped")
        ax[0].plot(s["t"], i_lo, color=ACCENT, lw=1, ls=":", label="pi01 injected")
        ax[0].plot(s["t"], i_hi, color=ACCENT2, lw=1, ls=":", label="pi02 injected")
        ax[0].legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8)
        ax[0].set_title(f"Servo actuator — cumulative resampler corrections {args.label}", color=FG, fontsize=11)
        ax[0].set_ylabel("samples (cum.)")
        ax[1].plot(s["t"], rate_lo, color=ACCENT, lw=0.7, label="pi01 drop/s")
        ax[1].plot(s["t"], rate_hi, color=ACCENT2, lw=0.7, label="pi02 drop/s")
        ax[1].legend(facecolor=BG, edgecolor=BORDER, labelcolor=FG, fontsize=8)
        ax[1].set_title("Drop rate (actuator effort) — settles as servo converges", color=FG, fontsize=11)
        ax[1].set_ylabel("samples/s"); ax[1].set_xlabel("time (min)")
        for a in ax:
            style(a)
        save(f, args.out, "actuator")
    else:
        print("actuator: too few synced polls")

    json.dump(summary, open(f"{args.out}_extras.json", "w"), indent=2)
    print("summary:", json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
