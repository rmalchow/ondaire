#!/usr/bin/env python3
"""Analyze a recording of reference.wav playback for chop/dropouts/pitch.

Usage: scripts/analyze-recording.py <recording.wav>

reference.wav (scripts make it via the play test) is a continuous 1 kHz sine
on the left + a 1 kHz tick at each whole second on the right. A clean playback
records as a solid 1 kHz tone with evenly-spaced ticks; this script reports the
dropout fraction, the gap/burst period (which, if it locks to bufferMs, means
our jitter buffer is underrun-storming), and the dominant frequency.
"""
import sys, wave, struct

def main(path):
    w = wave.open(path)
    rate, n, ch = w.getframerate(), w.getnframes(), w.getnchannels()
    s = struct.unpack("<%dh" % (n * ch), w.readframes(n))
    a = list(s[0::ch])
    dur = n / rate
    print(f"{path}: {dur:.1f}s  {rate}Hz  {ch}ch  frames={n}")
    try:
        import numpy as np
    except ImportError:
        print("(install numpy for full analysis)")
        return
    a = np.array(a, dtype=float)
    win = int(rate * 0.005)
    env = np.array([np.sqrt(np.mean(a[i:i+win]**2)) for i in range(0, len(a)-win, win)])
    if env.max() == 0:
        print("SILENT recording")
        return
    active_level = np.median(env[env > env.max() * 0.1])
    active = env >= active_level * 0.25
    print(f"active fraction (audible) = {100*active.mean():.0f}%   "
          f"=> {100*(1-active.mean()):.0f}% dropout")
    onsets = np.where(np.diff(active.astype(int)) == 1)[0]
    if len(onsets) > 2:
        periods = np.diff(onsets) * 5  # ms
        import collections
        hist = collections.Counter(int(p // 10) * 10 for p in periods)
        common = hist.most_common(3)
        print(f"burst period: median={np.median(periods):.0f}ms  "
              f"most-common bins(ms)={common}")
        print("  -> if the period tracks bufferMs, the jitter buffer is the oscillator")
    A = np.abs(np.fft.rfft(a * np.hanning(len(a))))
    f = np.fft.rfftfreq(len(a), 1/rate)
    k = int(np.argmax(A))
    print(f"dominant frequency = {f[k]:.0f}Hz  (reference tone = 1000Hz)")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
