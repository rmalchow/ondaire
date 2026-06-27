#!/usr/bin/env python3
"""
clock_telemetry.py — telemetry-only clock & playout-correction graph (no mic).

Polls GET /api/playback/statuses at ~1 Hz for N minutes, logs each poll as
"<epoch> <json-array>" (first data line "<epoch> PLAY" = t0, mirroring the rest
of the toolkit), then renders one brand-styled figure:

  Panel 1 — Device clocks vs the master (the master clock is, by definition, the
            reference: a flat line at 0). Each device's offsetNs (master−local,
            ms) shows how far its clock sits from the master and how its crystal
            wanders; each device's mean ratePPM (the servo's settled rate order)
            is the literal "how far off the crystal is", noted in the legend.
  Panel 2 — Playout corrections the servo applied to fight that drift: cumulative
            samples injected / dropped / silenced (ms), re-zeroed to the capture
            start so the slope is the accumulation during THIS run.

Counters in /api/playback/statuses are lifetime-cumulative; we subtract each
node's first observed value so the graph shows only this window. offsetNs is
absolute (not re-zeroed) — that IS the distance from the master.

  # capture live + graph (20 min, the two zero nodes):
  .venv/bin/python clock_telemetry.py --minutes 20 --nodes zero-01,zero-02 \
      --master http://192.168.71.63:8080 --out results/clocks

  # re-graph from a saved log without re-capturing:
  .venv/bin/python clock_telemetry.py --from-log results/clocks.jsonl \
      --nodes zero-01,zero-02 --out results/clocks
"""
from __future__ import annotations
import argparse, json, re, sys, time
from urllib.request import urlopen

SR = 48_000  # samples/s → samples/48 = ms (per channel)
BG, FG, MUTED, ACCENT, ACCENT2, ACCENT3, WARN, BORDER = (
    "#11151a", "#e6edf3", "#8b97a7", "#35e3b3", "#5bc8ff", "#ffb454", "#ff6b6b", "#2a3340")
HEXID = re.compile(r"^[0-9a-f]{32}$")


def get_json(url, timeout=5):
    with urlopen(url, timeout=timeout) as r:
        return json.load(r)


def resolve_nodes(master, names):
    """Map each name-or-id in `names` to (id, display_name) via /api/cluster."""
    cluster = get_json(master.rstrip("/") + "/api/cluster")
    by_name = {n["name"]: n for n in cluster["nodes"]}
    by_id = {n["id"]: n for n in cluster["nodes"]}
    out = []
    for tok in names:
        if tok in by_name:
            out.append((by_name[tok]["id"], tok))
        elif tok in by_id:
            out.append((tok, by_id[tok]["name"]))
        elif HEXID.match(tok):
            out.append((tok, tok[:8]))  # id we haven't seen in cluster yet
        else:
            sys.exit(f"unknown node {tok!r} — not a cluster name or 32-hex id")
    return out


def capture(master, minutes, interval, out_log):
    url = master.rstrip("/") + "/api/playback/statuses"
    deadline = time.time() + minutes * 60
    n = 0
    with open(out_log, "w") as f:
        f.write(f"{time.time():.3f} PLAY\n")
        f.flush()
        while time.time() < deadline:
            tick = time.time()
            try:
                arr = get_json(url)
                f.write(f"{tick:.3f} {json.dumps(arr, separators=(',', ':'))}\n")
                f.flush()
                n += 1
                if n % 30 == 0:
                    left = (deadline - tick) / 60
                    print(f"  …{n} polls, {left:.1f} min left", flush=True)
            except Exception as e:
                f.write(f"{tick:.3f} ERR {e}\n")
                f.flush()
            slp = interval - (time.time() - tick)
            if slp > 0:
                time.sleep(slp)
    print(f"captured {n} polls → {out_log}")


def parse(log, ids):
    """Return t0-anchored series per node id: t_min, offset_ms, ppm, inj/drop/sil ms."""
    t0 = None
    series = {i: dict(t=[], off=[], ppm=[], inj=[], drop=[], sil=[], synced=[]) for i in ids}
    for line in open(log):
        line = line.strip()
        if not line:
            continue
        sp = line.split(" ", 1)
        if len(sp) < 2:
            continue
        ts, rest = float(sp[0]), sp[1]
        if rest == "PLAY":
            t0 = ts
            continue
        if t0 is None or rest.startswith("ERR"):
            continue
        try:
            arr = json.loads(rest)
        except Exception:
            continue
        d = {n["nodeId"]: n for n in arr}
        for i in ids:
            n = d.get(i)
            if not n:
                continue
            s = series[i]
            s["t"].append((ts - t0) / 60.0)
            s["off"].append(n["offsetNs"] / 1e6)         # ms
            s["ppm"].append(n["ratePPM"])
            s["inj"].append(n["samplesInjected"] / 48.0)  # ms
            s["drop"].append(n["samplesDropped"] / 48.0)  # ms
            s["sil"].append(n["silence"] / 48.0)          # ms
            s["synced"].append(bool(n["synced"]))
    return series


def binned_rate(t_min, cum_ms, nbins):
    """Rate of a cumulative-ms counter, ppm (≡ µs/s), over ~equal time bins.

    Raw 1 Hz diffs of an integer-sample counter are badly quantised, so we diff
    the cumulative total across wider bins instead: rate = Δms/Δs · 1000 µs/ms.
    """
    import numpy as np
    t = np.asarray(t_min, float); c = np.asarray(cum_ms, float)
    if t.size < 2:
        return np.array([]), np.array([])
    edges = np.linspace(t.min(), t.max(), nbins + 1)
    ctr, rate = [], []
    for i in range(nbins):
        lo, hi = edges[i], edges[i + 1]
        m = (t >= lo) & (t <= hi) if i == nbins - 1 else (t >= lo) & (t < hi)
        if m.sum() < 2:
            continue
        ts, cs = t[m], c[m]
        dt_s = (ts[-1] - ts[0]) * 60.0
        if dt_s <= 0:
            continue
        ctr.append(0.5 * (lo + hi))
        rate.append((cs[-1] - cs[0]) / dt_s * 1000.0)  # ms/s → µs/s ≡ ppm
    return np.array(ctr), np.array(rate)


def binned_slope(t_min, y_ms, nbins):
    """Local slope of a (noisy) ms series, ppm (≡ µs/s), per ~equal time bin.

    For the clock offset: offsetNs ramps at the crystal's rate, so its raw value
    is cumulative — the meaningful, non-cumulative quantity is the SLOPE (how fast
    the device clock pulls away from the master), which IS the crystal ppm. A
    per-bin least-squares fit is robust to the RTT noise on each sample.
    """
    import numpy as np
    t = np.asarray(t_min, float); y = np.asarray(y_ms, float)
    if t.size < 2:
        return np.array([]), np.array([])
    edges = np.linspace(t.min(), t.max(), nbins + 1)
    ctr, slope = [], []
    for i in range(nbins):
        lo, hi = edges[i], edges[i + 1]
        m = (t >= lo) & (t <= hi) if i == nbins - 1 else (t >= lo) & (t < hi)
        if m.sum() < 3:
            continue
        # fit µs vs seconds → slope is µs/s ≡ ppm directly
        a = np.polyfit(t[m] * 60.0, y[m] * 1000.0, 1)[0]
        ctr.append(0.5 * (lo + hi)); slope.append(a)
    return np.array(ctr), np.array(slope)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--master", default="http://192.168.71.63:8080")
    ap.add_argument("--nodes", default="zero-01,zero-02", help="comma list of names or 32-hex ids")
    ap.add_argument("--minutes", type=float, default=20.0)
    ap.add_argument("--interval", type=float, default=1.0)
    ap.add_argument("--out", default="results/clocks")
    ap.add_argument("--from-log", default=None, help="re-graph this jsonl; skip live capture")
    args = ap.parse_args()

    names = [s.strip() for s in args.nodes.split(",") if s.strip()]
    log = args.from_log or (args.out + ".jsonl")

    if args.from_log:
        nodes = resolve_nodes(args.master, names)
    else:
        nodes = resolve_nodes(args.master, names)
        print(f"capturing {args.minutes:g} min from {args.master} "
              f"({', '.join(nm for _, nm in nodes)})")
        capture(args.master, args.minutes, args.interval, log)

    ids = [i for i, _ in nodes]
    label = {i: nm for i, nm in nodes}
    series = parse(log, ids)

    import numpy as np
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    plt.rcParams.update({"figure.facecolor": BG, "axes.facecolor": BG,
        "savefig.facecolor": BG, "text.color": FG, "axes.labelcolor": MUTED,
        "xtick.color": MUTED, "ytick.color": MUTED, "axes.edgecolor": BORDER,
        "axes.grid": True, "grid.color": BORDER, "grid.alpha": 0.5, "font.size": 10})
    colors = [ACCENT, ACCENT2, ACCENT3, WARN]

    span = max((max(s["t"]) - min(s["t"])) for s in series.values() if s["t"])
    nb = max(6, int(span * 60 / 60))       # ~60 s bins for the correction rates
    nb_clk = max(4, int(span * 60 / 150))  # ~150 s bins — the clock slope is RTT-noisy

    # Pre-compute per-node rates (ppm), kept for both figures. The clock "drift
    # rate" is the slope of offsetNs (its raw value is cumulative); the correction
    # rates are the slopes of the injected/dropped/silence counters.
    summary, plotd = {}, {}
    for i in ids:
        s = series[i]
        if not s["t"]:
            continue
        off = np.array(s["off"]); ppm = np.array(s["ppm"])
        tcr, rcr = binned_slope(s["t"], s["off"], nb_clk)
        rcr = -rcr  # offsetNs is master−local; negate → device-vs-master (fast = +ppm)
        ti, ri = binned_rate(s["t"], np.array(s["inj"]) - s["inj"][0], nb)
        td, rd = binned_rate(s["t"], np.array(s["drop"]) - s["drop"][0], nb)
        tsil, rsil = binned_rate(s["t"], np.array(s["sil"]) - s["sil"][0], nb)
        plotd[i] = dict(tcr=tcr, rcr=rcr, ti=ti, ri=ri, td=td, rd=rd, tsil=tsil, rsil=rsil)
        summary[label[i]] = dict(
            crystal_ppm_measured=float(np.mean(rcr)) if rcr.size else 0.0,
            servo_ppm_mean=float(np.mean(ppm)),
            offset_ms_mean=float(np.mean(off)), offset_ms_span=float(off.max() - off.min()),
            inj_rate_ppm=float(np.mean(ri)) if ri.size else 0.0,
            drop_rate_ppm=float(np.mean(rd)) if rd.size else 0.0,
            silence_rate_ppm=float(np.mean(rsil)) if rsil.size else 0.0,
            silence_ms_total=float(s["sil"][-1] - s["sil"][0]),
            polls=len(s["t"]))

    # ── Image 1 — how far off each crystal runs, as a RATE vs the master. The
    #    master clock is the reference (0 ppm); each device's line is the measured
    #    slope of its clock offset = its crystal's drift. (bare) ────────────────
    fig1, axc = plt.subplots(figsize=(11, 4.6), dpi=160)
    axc.axhline(0, color=FG, lw=1.5, label="master clock (study) — reference, 0 ppm")
    for k, i in enumerate(ids):
        if i not in plotd or not plotd[i]["rcr"].size:
            continue
        c = colors[k % len(colors)]
        axc.plot(plotd[i]["tcr"], plotd[i]["rcr"], color=c, lw=1.8,
                 label=f"{label[i]}  ({summary[label[i]]['crystal_ppm_measured']:+.1f} ppm)")
    axc.set_xlabel("time (minutes)")
    axc.set_ylabel("clock drift rate vs master (µs/s ≡ ppm)")
    axc.legend(loc="best", frameon=False, fontsize=9)
    fig1.tight_layout()
    fig1.savefig(args.out + "_clocks.svg"); fig1.savefig(args.out + "_clocks.png")

    # ── Image 2 — injection / drop RATE in ppm; the rate the servo applies to
    #    cancel each crystal. Faint line = the node's measured crystal ppm, so
    #    injected (minus the near-zero dropped) lands on it → net ≈ 0. (bare) ────
    fig2, axr = plt.subplots(figsize=(11, 4.6), dpi=160)
    axr.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    for k, i in enumerate(ids):
        if i not in plotd:
            continue
        d = plotd[i]; c = colors[k % len(colors)]
        axr.axhline(summary[label[i]]["crystal_ppm_measured"], color=c, lw=0.8, alpha=0.3)
        if d["ri"].size:
            axr.plot(d["ti"], d["ri"], color=c, lw=1.9, ls="-", label=f"{label[i]} injected")
        if d["rd"].size:
            axr.plot(d["td"], d["rd"], color=c, lw=1.6, ls="--", label=f"{label[i]} dropped")
    axr.set_xlabel("time (minutes)")
    axr.set_ylabel("correction rate (µs/s ≡ ppm)")
    axr.legend(loc="best", frameon=False, ncol=len(ids), fontsize=8)
    fig2.tight_layout()
    fig2.savefig(args.out + "_rates.svg"); fig2.savefig(args.out + "_rates.png")

    # ── Image 3 — silence on its own: ms of inserted silence per minute. It's a
    #    dropout duty (underruns), not a clock rate, so it gets its own axis and
    #    typically lives near zero once the buffers have filled. (bare) ─────────
    fig3, axs = plt.subplots(figsize=(11, 4.6), dpi=160)
    axs.axhline(0, color=MUTED, lw=0.8, alpha=0.5)
    for k, i in enumerate(ids):
        if i not in plotd or not plotd[i]["rsil"].size:
            continue
        d = plotd[i]; c = colors[k % len(colors)]
        # rsil is µs/s (ppm-equivalent); ×0.06 → ms of silence per minute.
        axs.plot(d["tsil"], d["rsil"] * 0.06, color=c, lw=1.6,
                 label=f"{label[i]} silence  ({summary[label[i]]['silence_ms_total']:.0f} ms total)")
    axs.set_xlabel("time (minutes)")
    axs.set_ylabel("silence inserted (ms per minute)")
    axs.legend(loc="best", frameon=False, ncol=len(ids), fontsize=8)
    fig3.tight_layout()
    fig3.savefig(args.out + "_silence.svg"); fig3.savefig(args.out + "_silence.png")

    json.dump(summary, open(args.out + ".json", "w"), indent=1)
    print(f"wrote {args.out}_{{clocks,rates,silence}}.{{svg,png}} + {args.out}.json")
    for nm, v in summary.items():
        print(f"  {nm}: crystal {v['crystal_ppm_measured']:+.2f} ppm measured "
              f"(servo {v['servo_ppm_mean']:+.2f}) | inject {v['inj_rate_ppm']:+.1f} ppm  "
              f"drop {v['drop_rate_ppm']:+.1f} ppm  silence {v['silence_ms_total']:.0f} ms total")


if __name__ == "__main__":
    main()
