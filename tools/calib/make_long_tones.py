#!/usr/bin/env python3
"""
make_long_tones.py — write an arbitrarily long gated dual-tone WAV in chunks,
without building the whole signal in memory. Same waveform as tones.generate
(L=fL, R=fR, shared raised-cosine gate, 2 s lead silence), continuous phase via
absolute sample index. For the long convergence captures (a 2 h file is ~1.4 GB).

Usage: python make_long_tones.py --minutes 120 --out /path/calib_tones_2h.wav
"""
from __future__ import annotations
import argparse, wave
import numpy as np
import tones  # FL, FR, SR, gate params

SR = tones.SR
CHUNK_S = 60  # write one minute at a time


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--minutes", type=float, default=120.0)
    ap.add_argument("--out", required=True)
    ap.add_argument("--lead-s", type=float, default=2.0)
    ap.add_argument("--amp", type=float, default=0.5)
    args = ap.parse_args()

    n_on, n_off, n_ramp = int(tones.ON_S * SR), int(tones.OFF_S * SR), int(tones.RAMP_S * SR)
    cyc = tones._gate(n_on, n_off, n_ramp)
    L = len(cyc)
    lead = int(args.lead_s * SR)
    total = int(args.minutes * 60 * SR) + lead
    w = wave.open(args.out, "wb")
    w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR)
    chunk = CHUNK_S * SR
    written = 0
    while written < total:
        n = min(chunk, total - written)
        idx = np.arange(written, written + n)
        gate = np.where(idx < lead, 0.0, cyc[(idx - lead) % L])
        l = args.amp * gate * np.sin(2 * np.pi * tones.FL * idx / SR)
        r = args.amp * gate * np.sin(2 * np.pi * tones.FR * idx / SR)
        st = np.empty(n * 2, dtype="<i2")
        st[0::2] = np.clip(l, -1, 1) * 32767
        st[1::2] = np.clip(r, -1, 1) * 32767
        w.writeframes(st.tobytes())
        written += n
    w.close()
    print(f"wrote {args.out} ({args.minutes} min, {total*4/1e9:.2f} GB, L={tones.FL:.0f} R={tones.FR:.0f} Hz)")


if __name__ == "__main__":
    main()
