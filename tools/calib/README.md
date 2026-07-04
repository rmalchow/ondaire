# Calibration & measurement toolkit (`tools/calib/`)

The **independent hardware arbiter** for ondaire's playout drift/sync work.
These are standalone Python tools that turn a single microphone (or a played
file plus the players' own telemetry) into ground-truth numbers about how well
the speakers are actually aligned in time — independent of what the Go servo
*thinks* is happening. The Go servo reports what it *believes* about each node's
clock; these tools measure what a microphone *actually hears* (and what the
resampler *actually did*), so the two can be cross-checked.

The acoustic methodology is also the reference DSP for the eventual Go port
(`internal/calib`), and the instrument used to validate the playout rate/phase
control described in `docs/architecture/playout-pipeline.md` and
`docs/architecture/clock-sync.md`. This
document stays on the measurement tools and deliberately does not describe the Go
servo internals.

Two measurement families live here:

1. **Acoustic arrival** (`sweep.py` / `analyze.py` / `selftest.py` /
   `plot_coherence.py`, plus `codec.py`). Players sound one at a time; an
   exponential sine sweep + Farina inverse filter + matched filter gives each
   speaker's acoustic arrival to a fraction of a sample → inter-speaker offset.
2. **Clock drift & servo convergence** (`tones.py`, `octave_drift.py`,
   `lr_drift.py`, the `make_*`/`run_*` capture tooling, and the
   `analyze_*`/`graph_*`/`compare_*` family). Long dual-tone / octave / sweep
   captures measure DAC-crystal drift in **ppm**, inter-speaker drift over time,
   per-channel (L/R) drift, and whether the proportional servo *converges*.

Everything below is derived from the scripts as written; flag names, defaults,
and file names are real.

## Contents

- [0. Units & constants](#0-units--constants)
- [1. Setup](#1-setup)
- [2. Validate the DSP (no hardware)](#2-validate-the-dsp-no-hardware)
- [3. Methodology 1 — acoustic arrival (sweep/coherence)](#3-methodology-1--acoustic-arrival-sweepcoherence)
- [4. Methodology 2 — clock drift & servo convergence](#4-methodology-2--clock-drift--servo-convergence)
- [5. Per-file reference](#5-per-file-reference)
- [6. WAV formats](#6-wav-formats)
- [7. Output artifacts (`results/`)](#7-output-artifacts-results)
- [8. Gotchas & notes](#8-gotchas--notes)

---

## 0. Units & constants

Everything is fixed at **48 kHz** (`SR = 48_000`), ondaire's canonical rate.

| Quantity | Value |
|----------|-------|
| 1 sample | 20.833 µs |
| 1 sample (sound travel @ 343 m/s) | 7.1 mm |
| 1 ms | 48 samples |
| 1 ppm (clock-rate error) | 1 µs/s ≡ 1 µs of accrued shift per second |

The ppm↔rate identity is load-bearing for the drift family: a clock running
`ppm` parts-per-million fast accrues `ppm` microseconds of timing shift every
second, so **the time-derivative of a measured offset (µs/s) is directly
comparable to a commanded ppm** — but the offset *itself* (a position, µs) is
not. `graph_servo_ppm.py` is built entirely around that distinction.

---

## 1. Setup

```bash
cd tools/calib
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt        # numpy, scipy, matplotlib
```

`requirements.txt` pins `numpy>=1.24`, `scipy>=1.10`, `matplotlib>=3.7` (the last
is used only by the plotting/graphing scripts). `.gitignore` excludes `.venv/`,
`*.wav`, `__pycache__/`, `*.pyc`, and the entire `results/` directory — do not
commit them (the 2 h capture WAV alone is ~1.4 GB); regenerate captures locally.

All examples below assume `.venv/bin/python`.

---

## 2. Validate the DSP (no hardware)

The three self-tests need no hardware and no network. Each synthesizes a signal
with known truth (offsets, echoes, noise), runs the *real* estimator, and exits
non-zero on failure:

```bash
.venv/bin/python selftest.py     # acoustic-arrival estimator vs known sub-sample offsets
.venv/bin/python tones.py        # dual-tone edge timing vs a known L↔R offset
.venv/bin/python codec.py        # in-band label codec round-trip + ADC-drift anchor fit
```

These self-tests are the contract — run all three after any DSP change. See
[Gotchas](#8-gotchas--notes) for the exact pass thresholds.

---

## 3. Methodology 1 — acoustic arrival (sweep/coherence)

### 3.1 Theory

Prove we can measure the **acoustic arrival time** of a sine-sweep burst from
each player using a **single microphone**, with players sounding **one at a
time** (time-interleaved). The honest target is sample-level relative offset,
sub-sample with sweep interpolation.

The probe is an **exponential (logarithmic) sine sweep** — instantaneous
frequency rises geometrically `f(t) = f0·(f1/f0)^(t/T)`. Its large
time-bandwidth product gives strong correlation processing gain, so the arrival
can be recovered even at low SNR with room echoes present. Defaults: 100 Hz →
12 kHz over 1.0 s with 10 ms raised-cosine fades (fades prevent the start/stop
click that would smear the correlation peak).

Two independent estimators run on the same recording and must agree:

- **Matched filter** = cross-correlation = `recording (*) reference[::-1]`
  (`scipy.signal.fftconvolve`, `mode="full"`). The peak lag is the arrival; lag 0
  lands at output index `len(ref)-1`, so `arrival = peak_index − (len(ref)−1)`.
- **Farina deconvolution** — convolve the recording with the **inverse filter**
  (the time-reversed sweep with a +6 dB/oct amplitude envelope that flattens the
  spectrum) to recover the room impulse response; its peak is the arrival. The
  envelope changes amplitude shaping, *not* the alignment lag, so the same
  `len(ref)−1` offset applies.

Both peaks are refined to **sub-sample precision by parabolic (3-point vertex)
interpolation**: `δ = 0.5·(y[-1]−y[+1]) / (y[-1]−2·y[0]+y[+1])`, true peak =
`k + δ`, with the flat-top (zero-denominator) and array-edge cases guarded.

### 3.2 Lab procedure

1. **Generate the reference sweep:**
   ```bash
   .venv/bin/python sweep.py --out ref.wav
   ```
   Default: 100 Hz → 12 kHz, 1.0 s, 48 kHz, mono float32, 10 ms fades, amp 0.5.
   `--inv-out inv.wav` also writes the (storage-normalised) inverse filter.

2. **Start one continuous single-mic recording** at 48 kHz:
   ```bash
   arecord -f S16_LE -c 2 -r 48000 capture.wav
   ```

3. **Play the sweep on player A, settle ~1 s, then on player B** (one at a time;
   the ~1 s settle gap is what lets the energy auto-detector split the bursts).

4. **Stop recording**, note roughly which sample range each burst occupies.

5. **Analyze:**
   ```bash
   # auto-detect bursts by energy:
   .venv/bin/python analyze.py capture.wav ref.wav

   # or give explicit windows (more reliable) — start:end sample pairs:
   .venv/bin/python analyze.py capture.wav ref.wav \
       --windows 48000:110000,150000:212000 --labels A,B
   ```
   Prints per-player arrival (matched filter and IR — they should agree to a
   fraction of a sample, shown as `xc-IR Δ`) and the inter-player offset vs
   player A in **samples, µs, and ms**. Prefer explicit `--windows` for precise
   work; auto-detection assumes a clear (~1 s) settle gap.

### 3.3 Periodic-sweep stability (`plot_coherence.py`)

For a *continuous* recording of a group looping one sweep file every `--cadence`
seconds (not interleaved — coherence of a single stream), `plot_coherence.py`:

1. matched-filters every sweep to a sub-sample arrival,
2. fits the best constant-rate clock (`arrival = rate·index + offset`, robust
   least-squares with one 4σ residual purge),
3. reports the residual as the per-sweep timing error in µs (**this residual IS
   the sync jitter**; the fitted rate is the constant crystal offset between the
   ondaire clock and the mic ADC — not a defect),
4. renders a brand-styled jitter graph (SVG + PNG) with a marginal histogram,
5. dumps the points as JSON.

```bash
.venv/bin/python plot_coherence.py --rec run5min.wav --cadence 2.5 --discard 4 \
    --out results/coherence_5min --title "Acoustic sync stability"
```

Headline outputs: **RMS jitter (µs)**, p95, peak, sweeps-used, span, and the
**rate offset (ppm)**.

### 3.4 In-band labelling & ADC anchoring (`codec.py`)

A long acoustic run leaks the mic ADC's own crystal drift into any *per-speaker
absolute* number (the midpoint trick only cancels it for the *differential*
inter-speaker measurement). `codec.py` is the "self-identifying sweep" layer:
after each sweep it appends a short, slow, reverb-tolerant **tone frame**
carrying a rolling 16-bit counter and, every Nth sweep, an absolute
**master-time epoch** (master-clock ms). Decoupling: precision still comes from
the sweep peak; identity/time come from the frame, which only needs to be
*readable*.

Frame layout: `[SYNC 1000 Hz] [digit…] [checksum]`, each a 40 ms pure tone in a
16-tone alphabet (1500 Hz + 150 Hz·digit → 4 bits/slot) with 10 ms guards;
decoded by single-bin (Goertzel-equivalent) power with a checksum gate. The
anchors `(recorded_sample_position, master_time)` are fit with `np.polyfit`
(deg-1 removes constant ADC rate error, deg-2 also removes slow thermal
curvature); the constant term absorbs capture-start + reference-DAC latency +
propagation, so only the *shape* matters. **Caveat baked into the docstring:**
anchors must be emitted by a single in-room reference node (ideally the master),
never by the speakers under test, or their DAC drift folds into the reference.

`make_playout.py --coded` is the only producer that bakes counter frames into a
playout file; the decode/anchor path is currently exercised by `codec.py`'s own
self-test, not by the drift analyzers.

### 3.5 Validation against the system's own delay control

This proves the acoustic measurement and ondaire's playout-delay control agree:

1. Measure the A→B offset at default delay (`O0`).
2. Inject a known delay on B via its **`outputDelayMs`** in the UI.
3. Re-measure: the new offset should be `O0 + outputDelayMs` within ~1–2
   samples (e.g. 5 ms ≈ 240 samples).
4. Repeat to report the noise floor (mean ± stddev).

The `selftest.py` DSP-intrinsic error is well under a sample (< 0.02 sample even
at low SNR with echoes); real-world spread comes from room, mic, and clock.

---

## 4. Methodology 2 — clock drift & servo convergence

### 4.1 Theory

Each Raspberry Pi DAC has a free-running crystal that drifts (rate error + slow
thermal curvature). The playout servo trims each node's effective rate (in ppm)
to hold sync. This family answers three questions with a microphone:

- **What is each speaker's drift vs the recording clock?** (the shared ~ppm mic
  offset + servo wiggle)
- **What is the inter-speaker drift?** (pi02 − pi01, with the common mic-clock
  rate cancelled — the coherence that actually matters)
- **Does the proportional servo converge** — ppm plateau, queue error settle,
  acoustic drift flatten — and is a residual ramp *thermal* (real physical
  drift) or a *per-session servo transient*?

Three signal designs, each trading robustness against measurement density:

- **Gated dual tones** (`tones.py`): both speakers play *continuously and
  simultaneously* at close mid-band frequencies (**fL = 2300 Hz on L/pi01,
  fR = 2900 Hz on R/pi02**), gated on/off together (0.40 s on / 0.40 s off, 6 ms
  raised-cosine edges). A bandpass (±250 Hz, 6th-order, 100 Hz guard) isolates
  each speaker; the **rising edge** of each burst is the direct arrival (robust
  to reverb, which only smears the *tail*). The L↔R edge offset is the
  inter-speaker error, sampled once per gate cycle (~1.25 Hz). The close, mid-band
  frequencies keep edge SNR symmetric while all harmonics/intermod products
  (600, 1700, 3500, 4000, 5200 Hz …) miss both passbands.
- **Octave-separated tones + sweeps** (`octave_drift.py`): each speaker repeats
  `tone — gap — sweep — gap` in a disjoint, **above-modal** band (≳1 kHz, so
  standing waves don't corrupt phase). Drift is tracked by **carrier phase** —
  heterodyne each band to baseband, unwrap phase densely *within* each tone
  window (per-sample change tiny → no slips), stitch across gaps — with the sweep
  arrival as a coarse cross-check. Phase tracking is the finest method here.
- **Wideband L/R interleave** (`lr_drift.py` + `make_playout.py`): both speakers
  play the *same* wideband sweep but in alternating time slots (pi01/L at
  t = 0, 2.4, 4.8 s …; pi02/R at t = 1.2, 3.6 s …), so they never overlap and each
  is identified by slot. Modal-free wideband matched filter, no phase unwrap —
  "the method that nailed the 46 cm move and a ±150 µs baseline."

**Why the inter-speaker number cancels the mic clock.** In all three, the mic
ADC's own rate is common to both speakers. `lr_drift.py` references each pi02 (R)
sweep to the **midpoint of its bracketing pi01 (L) sweeps**; `octave_drift.py`
subtracts the two bands' drift; `tones.py` differences the two edges. The common
playback-vs-mic rate cancels exactly, leaving the pi01↔pi02 coherence drift.

### 4.2 Capture workflows (`run_*.sh`)

All four scripts follow the same shape: truncate a stats log, start `arecord`
(stereo s16le @ 48 kHz) **first** so the onset is captured (the played files
carry a 2 s lead of silence for margin), `POST /api/play` a file URI, immediately
write a `<epoch> PLAY` marker to the log (this is **t0** for every analyzer),
then poll `GET /api/playback/statuses` at ~1 Hz with epoch timestamps until the
recording ends, then `POST /api/stop`. Master is hardcoded at
`http://192.168.71.63:8080`; outputs land in `tools/calib/results/`.

| Script | Duration | Sessions | Played file | Outputs | Purpose |
|--------|----------|----------|-------------|---------|---------|
| `run_tones.sh` | ~10 min (record 615 s, 610 polls) | 1 | `file:calib_tones.wav` | `tones_run.wav`, `tones_stats.jsonl` | Standard single dual-tone run for `analyze_servo.py` / `graph_servo_ppm.py`. |
| `run_tones_2h.sh` | ~2 h (record 7260 s, 7230 polls) | 1 uninterrupted | `file:calib_tones_2h.wav` | `tones_2h.wav` (~1.4 GB), `tones_stats_2h.jsonl` | Unattended P-only servo **convergence** baseline for `analyze_2h.py`. |
| `run_tones_split.sh` | 2 × ~3 min (record 185 s, 180 polls each) | 2, with `stop → sleep 1 → play` between | `file:calib_tones.wav` | `tones_split1/2.wav`, `tones_stats_split1/2.jsonl` | **Thermal vs servo-transient** test for `graph_split.py`. |
| `run_3min.sh` | ~3 min (record 190 s, 180 polls) | 1 | `file:calib_tones.wav` | `tones_v0150.wav`, `tones_stats_v0150.jsonl` | Minimal per-version smoke capture. |

The played `calib_tones*.wav` files are produced by `make_long_tones.py` (or
`tones.py --gen`) and must already exist in the master's media directory.

### 4.3 Analysis & graphing tools

The stats log lines are `"<epoch> <json-array>"` where the array is the
`/api/playback/statuses` payload; every parser anchors `t` at the `PLAY` marker,
keys nodes by `nodeId`, and keeps only polls where **both** nodes report
`synced: true`. The relevant per-node fields are `ratePPM`, `offsetNs`,
`phaseErrNs`, `deviceDelayNs`, `samplesInjected`, `samplesDropped`.

**`analyze_servo.py` — does the servo's realized correction match what it
commanded, and does either explain the acoustic reality?** Three views of one
run:

- **COMMANDED** = `∫ ratePPM·dt` per node (cumulative ms; `ppm/1e6 · dt`).
- **REALIZED** = `(samplesInjected − samplesDropped)/48` ms per node. The
  resampler is strictly 1-frame-in/1-frame-out, so the only way it nets samples
  in/out over the run is the carry overflow-drop / underflow-inject guard — that
  count *is* the rate correction the DAC actually saw.
- **ACOUSTIC** = the mic inter-speaker offset (R−L) from `tones.analyze`.

Prints commanded/realized spans, per-node `corr(commanded, realized)`, final
inj/drop counts, acoustic span, and `corr(acoustic, realized-difference)`; two
stacked panels + JSON.

```bash
.venv/bin/python analyze_servo.py --wav results/tones_run.wav \
    --stats-log results/tones_stats.jsonl \
    --pi-low <nodeId-L> --pi-high <nodeId-R> --out results/servo
```

**`graph_servo_ppm.py` — rate vs rate.** Because ppm is a rate, the commanded
ppm is compared to the **time-derivative** of the measured offset, not the offset
itself. Three panels: (1) commanded ppm per node + difference; (2) the measured
acoustic offset (R−L), MAD-de-spiked, binned to a 20 s robust median, with its
linear trend; (3) `d(offset)/dt` (µs/s ≡ ppm) overlaid on the commanded ppm
difference. If the servos fully cancel each DAC's drift the measured *rate* sits
near 0 even while each node commands tens of ppm. `--skip-min` (default 1.0)
drops the settling transient.

```bash
.venv/bin/python graph_servo_ppm.py --wav results/tones_run.wav \
    --stats-log results/tones_stats.jsonl \
    --pi-low <id> --pi-high <id> --out results/servo_ppm
```

**`analyze_2h.py` — convergence.** The 1.4 GB WAV is too big for one Hilbert
transform, so the acoustic offset is computed in **windows** read by byte offset
(default: a 60 s window every 120 s, `tones.analyze` each → MAD-filtered median).
Stats are read whole. Three panels (commanded ppm, device-queue error
`phaseErr`, windowed acoustic offset) over the full run; prints first-5-min vs
last-10-min ppm (settled?) and first-10-min vs last-30-min acoustic drift slope.
`--wav`/`--stats-log` default to the 2 h artifact names.

```bash
.venv/bin/python analyze_2h.py --pi-low <id> --pi-high <id>
```

**`graph_split.py` — thermal vs re-convergence.** Plots the two split captures on
one timeline (run2 drawn after run1 with a small visual gap and a "full restart"
marker). Top: commanded ppm per node — the servo resets on restart so run2
starts near 0; the question is whether it ramps to the **same** level as run1
(static crystal offset, slow servo re-converging) or **higher** (physical drift
accumulated during the gap → thermal). Bottom: acoustic offset, each run
re-zeroed to its own start so the **slope** is comparable. Reads the
`results/tones_{split1,split2}.{wav,jsonl}` files by `--run1`/`--run2` tag.

```bash
.venv/bin/python graph_split.py --pi-low <id> --pi-high <id> --out results/split
```

**`compare_drift.py` — mic vs telemetry.** Overlays the inter-speaker drift JSON
from `lr_drift.py` (`--mic-json`) against the players' self-reported clock-offset
difference (`(hi.offsetNs − lo.offsetNs)`) from a stats log. Both are detrended;
reports the correlation on a common grid, the `deviceDelayNs` difference (static
hardware, explaining the baseline acoustic offset), and per-node servo ppm.

```bash
.venv/bin/python compare_drift.py --mic-json results/wideband.json \
    --stats-log /tmp/stats_log.jsonl --pi-low <id> --pi-high <id> --out results/compare
```

**`lr_drift.py`** (wideband interleave) and **`octave_drift.py`** (octave bands)
each emit two graphs + JSON: `<out>_vsclock.*` (one speaker's arrivals detrended
of cadence → drift vs the mic clock, with the fitted ppm) and
`<out>_interspeaker.*` (the mic-clock-cancelled pi02−pi01 coherence drift, with
its RMS). Both classify/track robustly against missed sweeps (`lr_drift.py`
classifies L vs R by **cycle phase**, not detection-order parity, so one missed
sweep can't flip the sign of the whole tail) and purge reverb mis-picks with a
rolling-median outlier reject. `lr_drift.py --bare` omits the baked-in titles so
the marketing site can overlay its own brand text.

```bash
.venv/bin/python make_playout.py --minutes 30 --out /media/lrrun.wav --ref /tmp/ref_wb.npy
.venv/bin/python lr_drift.py --rec lrrun.wav --ref /tmp/ref_wb.npy --period 2.4 --discard 4 \
    --out results/wideband
.venv/bin/python octave_drift.py --rec octrun.wav --lo-tone 1200 --hi-tone 3200 \
    --lo-band 1000,1600 --hi-band 2600,4200 --period 7.5 --discard 6 --out results/octave2
```

---

## 5. Per-file reference

`I→O` lists the principal inputs → outputs. All drift graphers also write a
sibling `.json`. Node IDs (`--pi-low`/`--pi-high`) are the master's `nodeId`s for
the L (pi01) and R (pi02) speakers. **Channel assignment is done in physical
wiring** — which node plays L (the 2300 Hz tone) vs R (the 2900 Hz tone) is fixed
by how the speakers are cabled, not by any software config or API setting. There
is no channel field to query; map `nodeId` → L/R from the known wiring of the rig
(here pi01 = L = `--pi-low`, pi02 = R = `--pi-high`).

| File | Family | Key CLI flags | I → O | How to run (one line) |
|------|--------|---------------|-------|-----------------------|
| `sweep.py` | acoustic | `--out --inv-out --f0 --f1 --dur --rate --fade --amp --s16` | — → `ref.wav` (mono float32, opt. inverse filter) | `python sweep.py --out ref.wav` |
| `analyze.py` | acoustic | `recording reference --windows --labels` | recording WAV + ref WAV → printed offset table | `python analyze.py capture.wav ref.wav --windows 48000:110000,150000:212000 --labels A,B` |
| `selftest.py` | acoustic | *(none)* | synthetic recording → PASS/FAIL (exit code) | `python selftest.py` |
| `codec.py` | acoustic | *(none; self-test)* | synthetic frames → decode % + anchor-fit RMS, PASS/FAIL | `python codec.py` |
| `plot_coherence.py` | acoustic | `--rec --ref --sweep-dur --cadence --discard --out --title --subtitle` | periodic-sweep WAV → jitter graph (svg/png) + json | `python plot_coherence.py --rec run5min.wav --cadence 2.5 --discard 4 --out results/coherence_5min` |
| `make_playout.py` | drift | `--minutes --period --gap --f0 --f1 --dur --amp --lead --coded --ndig --out --ref` | — → stereo s16 playout WAV + `.npy` reference | `python make_playout.py --minutes 30 --out /media/lrrun.wav --ref /tmp/ref_wb.npy` |
| `lr_drift.py` | drift | `--rec --ref --period --gap --discard --out --label --bare` | interleave WAV + `.npy` ref → `_vsclock.*` + `_interspeaker.*` + json | `python lr_drift.py --rec lrrun.wav --ref /tmp/ref_wb.npy --period 2.4 --out results/wideband` |
| `octave_drift.py` | drift | `--rec --lo-sweep --hi-sweep --lo-band --hi-band --lo-tone --hi-tone --period --discard --out --label` | octave WAV + two `.npy` sweep refs → `_vsclock.*` + `_interspeaker.*` + json | `python octave_drift.py --rec octrun.wav --period 7.5 --discard 6 --out results/octave2` |
| `tones.py` | drift | `--gen --minutes --analyze --selftest` | — / dual-tone WAV → generated WAV / printed offset stats / PASS-FAIL | `python tones.py --gen calib_tones.wav --minutes 10` |
| `make_long_tones.py` | drift | `--minutes --out --lead-s --amp` | — → long gated dual-tone stereo WAV (chunked) | `python make_long_tones.py --minutes 120 --out calib_tones_2h.wav` |
| `analyze_servo.py` | drift | `--wav --stats-log --pi-low --pi-high --out --label` | dual-tone WAV + stats log → commanded/realized/acoustic graph + json | `python analyze_servo.py --wav results/tones_run.wav --stats-log results/tones_stats.jsonl --pi-low <id> --pi-high <id>` |
| `graph_servo_ppm.py` | drift | `--wav --stats-log --pi-low --pi-high --out --label --skip-min` | dual-tone WAV + stats log → ppm-vs-offset-rate graph + json | `python graph_servo_ppm.py --wav results/tones_run.wav --stats-log results/tones_stats.jsonl --pi-low <id> --pi-high <id>` |
| `analyze_2h.py` | drift | `--wav --stats-log --pi-low --pi-high --out` | 2 h WAV (windowed) + stats log → convergence graph + json | `python analyze_2h.py --pi-low <id> --pi-high <id>` |
| `graph_split.py` | drift | `--pi-low --pi-high --run1 --run2 --out` | two split WAVs + stats logs → restart-comparison graph | `python graph_split.py --pi-low <id> --pi-high <id> --out results/split` |
| `compare_drift.py` | drift | `--mic-json --stats-log --pi-low --pi-high --out --label --bare` | `lr_drift` json + stats log → mic-vs-telemetry overlay + json | `python compare_drift.py --mic-json results/wideband.json --stats-log /tmp/stats_log.jsonl --pi-low <id> --pi-high <id>` |
| `run_tones.sh` | capture | *(edit vars in file)* | — → `tones_run.wav` + `tones_stats.jsonl` | `bash run_tones.sh &` |
| `run_tones_2h.sh` | capture | *(edit vars in file)* | — → `tones_2h.wav` + `tones_stats_2h.jsonl` | `bash run_tones_2h.sh &` |
| `run_tones_split.sh` | capture | *(edit vars in file)* | — → `tones_split{1,2}.*` | `bash run_tones_split.sh &` |
| `run_3min.sh` | capture | *(edit vars in file)* | — → `tones_v0150.*` | `bash run_3min.sh &` |

---

## 6. WAV formats

- **Reference sweep** (`sweep.py`): 48 kHz, mono, **32-bit float**
  (`WAVE_FORMAT_IEEE_FLOAT`). `--s16` writes mono s16le instead.
- **Acoustic-arrival recording** (`analyze.py`): any rate (48 kHz expected),
  mono or stereo, PCM s16/s32/u8 or 32-bit float; stereo is folded to mono.
- **Drift captures**: stereo s16le @ 48 kHz with a 44-byte header (raw
  `arecord` / `pw-record` output). The drift tools read by byte offset.

---

## 7. Output artifacts (`results/`)

The directory is gitignored; it accumulates four kinds of file.

- **`*.wav`** — raw stereo s16le @ 48 kHz mic captures (44-byte header). The drift
  tools read these by byte offset, so a not-yet-finalized header is tolerated
  (`analyze_2h.read_window` uses the file size, not the header length field). The
  2 h capture is ~1.4 GB.
- **`*.jsonl`** — the 1 Hz stats poll. Each line is `"<epoch> <json>"`; the first
  data line is `"<epoch> PLAY"` (the t0 anchor). The JSON is the
  `/api/playback/statuses` array.
- **`*.png` / `*.svg`** — brand-styled graphs (dark `#11151a` background, accent
  `#35e3b3`/`#5bc8ff`). SVG for the marketing site, PNG for previews. Naming
  follows the `--out` prefix, with the drift tools appending `_vsclock` /
  `_interspeaker`.
- **`*.json`** — machine-readable results that mirror each graph. Common shapes:
  - **coherence** (`plot_coherence.py`): `rms_jitter_us`, `p95_us`, `peak_us`,
    `rate_offset_ppm`, `sweeps_used`, `span_minutes`, and a `points[]` list of
    `{t_min, err_us}`.
  - **drift** (`lr_drift.py`, `octave_drift.py`): top-level `ppm`, a `vsclock`
    block (`t_min[]`, `drift_us[]`), and an `interspeaker` block (`t_min[]`,
    `drift_us[]`, `rms_us`).
  - **servo** (`analyze_servo.py`, `graph_servo_ppm.py`): commanded/realized ms
    series per node, the acoustic series, `corr_*`, `drift_ppm`, and the final
    inj/drop counts.

Representative existing artifacts: `coherence_5min.*`, `wideband.*`,
`octave2.*`, `run30.*` (acoustic + drift); `servo.*`, `servo_ppm*.*`,
`conv_2h.*`, `split.*`, `compare.*` (servo/convergence); the `_wired`/`_wifi`,
`_v0140`/`_v0150` suffixes are A/B captures across transports and firmware
versions.

---

## 8. Gotchas & notes

- **Self-tests are the contract.** `selftest.py` recovers known sub-sample
  offsets to < 0.02 sample at 30 dB SNR with two echoes (hard pass: 1 sample).
  `tones.py`'s self-test injects a 2500 µs L↔R offset and must recover the median
  within 200 µs with < 200 µs jitter. `codec.py` requires ≥ 98% frame decode at
  ~20 dB SNR and corrected-master-time RMS ≤ 8 µs through a drifting ADC. Run all
  three after any DSP change.
- **`tones.py` with no args runs the self-test** (not a no-op). Use `--gen` to
  produce a WAV, `--analyze` to read one.
- **Settle gap matters for acoustic auto-detect.** `analyze.py`'s energy detector
  assumes a clear ~1 s silence between interleaved bursts; for precise work pass
  explicit `--windows`. The detector deliberately does *not* pad windows by a
  full sweep length (it would overrun into the next burst).
- **Above-modal requirement.** `octave_drift.py` carrier-phase tracking needs the
  bands ≳1 kHz; below the room's modal region standing waves corrupt the phase.
  Tone windows exclude the sweep segment (which sweeps *through* the tone
  frequency) and the silent gaps.
- **Phase classification beats parity.** `lr_drift.py` labels sweeps L/R by cycle
  phase, not detection order, specifically so a single missed sweep doesn't flip
  the sign of the entire tail (a real failure seen on a ~10 min break).
- **Don't fit against derived quantities.** `lr_drift.py` fits the vs-clock trend
  against the *nominal* integer cycle number `round(ci)`, not the measured `ci`
  (which is derived from the arrivals — fitting against it would be circular,
  residual ≡ 0).
- **Mic ADC drift only cancels differentially.** Per-speaker absolute timing over
  a long run carries the mic's own ppm; only the inter-speaker (differenced or
  midpoint-referenced) number is mic-clock-free. The `codec.py` anchor layer
  exists to remove it for the absolute case, but is not yet wired into the drift
  analyzers.
- **Hardcoded master IP.** All `run_*.sh` scripts hardcode
  `http://192.168.71.63:8080` and expect `calib_tones*.wav` to already be in the
  master's media directory. Edit the variables at the top for a different rig.
- **Big files, gitignored.** `.venv/`, `*.wav`, and `results/` are never
  committed; regenerate captures locally.

### Things that look stale / unverified

- The playout rate/phase control this tool validates is documented in
  `docs/architecture/playout-pipeline.md` and `docs/architecture/clock-sync.md`.
- `make_playout.py --coded` and the whole `codec.py` decode/anchor path have no
  consumer in the drift analyzers — `codec.py` is validated only by its own
  self-test. It is staged reference DSP for the Go port, not part of any current
  capture→analysis pipeline.
- `octave_drift.py` defaults `--lo-sweep`/`--hi-sweep` to `/tmp/oct2_loS.npy` /
  `/tmp/oct2_hiS.npy`, but no script in this directory produces those octave-band
  sweep `.npy` references (unlike `make_playout.py`, which writes the wideband
  `.npy` for `lr_drift.py`). The octave capture/reference generator lives outside
  the toolkit or was ad hoc.
- `run_3min.sh` writes `tones_v0150`-named artifacts and is effectively a
  single-version smoke variant of `run_tones.sh` with the values inlined rather
  than parameterised; the `results/` `_wired`/`_wifi`/`_v0140` artifacts imply
  other ad-hoc capture variants that aren't checked-in scripts.
</content>
</invoke>
