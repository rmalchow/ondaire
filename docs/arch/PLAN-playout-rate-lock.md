# PLAN — closing the playout rate loop (servo redesign)

Status: design, not yet implemented. Motivated by the 2 h convergence capture
(`tools/calib/results/conv_2h.png`): the proportional servo ramps its commanded
ppm linearly to the ±300 clamp and never converges, while the device queue drains
to −96 ms in parallel. The loop is **open** — its actuator does not move its sensor.

## 1. Root cause

The playout has two clocks: the **master** clock (frame PTS, network-paced) and the
local **DAC** crystal (drifts ±tens of ppm vs master). Two things must be true:

- **Phase**: each frame's content must hit the speaker at `MasterToLocal(pts +
  buffer − delayOffset + equalize)`. This is the cross-node sync, and it WORKS —
  the deadline scheduler in `playoutLoop` re-pins every frame's start to the
  master clock (`sink.go`, `target`/`local`).
- **Rate**: samples must reach the DAC at the DAC's true rate, or the device queue
  drifts until it under/overruns.

The current `rateServo` tries to fix **rate** by measuring the **device-queue
depth** and actuating the **resample ratio**. But the loop writes a *fixed*
`FrameSamples` (960) per slot at master cadence, so samples/sec to the DAC =
`960 × slotRate` **regardless of the resample ratio**. The ratio changes *which*
input maps to those 960 outputs (content time-warp), never *how many* samples per
second reach the DAC. So the actuator has **zero authority** over the queue depth
it measures → the proportional term integrates a never-closing error → linear
ramp to the clamp. (v0.14.0 made the resampler *honestly apply* the command, which
is why pi01 then dropped 1.29 s of content over 2 h — the change exposed, did not
cause, the runaway.)

## 2. The correct decomposition (rate vs phase)

Keep the working **phase** machinery; fix the **rate** loop's actuator.

- **Phase / cross-node sync — UNCHANGED.** The deadline scheduler still schedules
  each frame's start at `MasterToLocal(pts + buffer − delayOffset + equalize)`. Do
  NOT replace it with a free-running DAC-pull (see §5): the deadlines ARE the
  multi-room phase alignment, and they already work.
- **Rate / DAC drift — NEW actuator.** The only way to change samples/sec to the
  DAC under a master-paced loop is to change the **number of output samples per
  slot**. So the rate servo commands an *average output length* `FrameSamples + δ`,
  and the resampler produces exactly that many samples by resampling the jitter
  buffer input. Writing `960 + δ` samples/slot makes write-rate = `(960+δ)·slotRate`;
  the servo drives `δ` so that equals the DAC rate → the queue holds. **Now the
  actuator (output length) directly moves the sensor (queue depth): the loop
  closes.**

`δ` is tiny and fractional: matching a +30 ppm DAC needs `δ = 960·30e-6 ≈ 0.029`
samples/slot — i.e. write 961 instead of 960 about once every 35 slots. That is
exactly the single-sample 2→3 / 3→2 correction the v0.14.0 resampler already does;
we're just letting the *output count* float instead of pinning it at 960.

This is the user's model: "check the output queue, pull that many samples through
the resampler" = the variable output length; "read the right packet vs the clock"
= the deadline scheduler already picking `seq` by PTS.

## 3. The error term

Work in **samples** (1 sample = 1/48000 s). Per slot, after reading the live device
queue depth `dd` (samples, from `DeviceDelay()`):

```
queueErr = ddEMA − setpoint          // samples; >0 ⇒ queue too deep ⇒ DAC slower than we feed
```

`setpoint` and the `ddEMA` smoothing (QueueTau) are kept from today. Sign: a queue
DEEPER than setpoint means we are feeding faster than the DAC drains ⇒ we must
produce FEWER samples ⇒ negative `δ`. So `δ` is driven *negatively* by `queueErr`.

## 4. The control law (PI, in output-sample units)

```
// gains (ppm-equivalent; tune empirically against the 2 h capture)
Kp  : output-samples per slot per sample of queueErr   (proportional)
Ki  : output-samples per slot per (sample·second) of ∫queueErr   (integral)

integral += queueErr * dtSec
// anti-windup: clamp the integral's authority to the rate clamp, and freeze it
// while uncalibrated / unsynced / output-clamped
maxI = clampSamples / Ki
integral = clamp(integral, −maxI, +maxI)

deltaRate = −(Kp*queueErr + Ki*integral)        // fractional output-samples per slot
deltaRate = clamp(deltaRate, −clampSamples, +clampSamples)   // ±0.03%·960 ≈ ±0.3 samp/slot is plenty

// realize the fractional rate as whole output samples with a carry:
carry += deltaRate
outLen = FrameSamples + trunc(carry)
carry  -= trunc(carry)
```

The **integral belongs here** — unlike the broken loop, this actuator controls its
sensor, so PI converges fast and parks `queueErr` at **zero** (no proportional
droop, no ramp). The earlier retraction was specific to the open loop.

`clampSamples`: a real crystal is < 100 ppm ⇒ `δ < 0.1` samples/slot; clamp to a
few tenths/slot so a transient can't command an audible rate step.

## 5. Why NOT full DAC-pull

A pure DAC-pull loop (drop the deadline sleep; the blocking `Write` paces
production; lock phase via read-position-vs-clock) is the textbook form and also
correct. We reject it here because:
- the deadline scheduler is the **multi-room phase alignment** and is proven; a
  DAC-pull rewrite would have to re-derive cross-node phase lock as a continuous
  loop, risking the one thing that currently works;
- the bug is purely the **rate actuator**, and §2 fixes exactly that with far less
  surgery on the realtime path.

So: keep deadlines for phase, add variable output length for rate.

## 6. Code impact

- `internal/sink/resampler.go` — `process` gains a target output length: produce
  `outLen` samples (step the cursor `outLen` times) instead of a fixed
  `FrameSamples`. The carry/trim logic is unchanged; the inj/drop counters keep
  counting single-sample corrections (now they ARE the realized rate match).
- `internal/sink/servo.go` — replace the ppm-ratio output with the PI law of §4
  returning `deltaRate` (output-sample units); add `integral` + anti-windup; reset
  clears it. `RatePPM` telemetry becomes `deltaRate/FrameSamples·1e6` for continuity.
- `internal/sink/sink.go` playout loop — `outLen := rs.targetLen()`; write the
  variable-length frame; everything else (deadline, jitter pop by `seq`, gen/reset)
  unchanged. Output buffer `out` must size to `FrameSamples+maxδ`.
- Unchanged: clock follower, jitter buffer + PTS, deadline scheduling, cross-room
  equalize, the single-sample resample primitive.

## 7. Validation

- Unit: servo drives a simulated drifting queue to setpoint and HOLDS it (no ramp);
  resampler emits the requested `outLen` with bounded carry and no seam glitch.
- Integration: re-run the 2 h capture. Expected: `queueErr`/phaseErr settles near 0
  within seconds and stays flat (no ramp to clamp); commanded `δ` plateaus at the
  crystal offset; acoustic inter-speaker drift → ~0 after lock (vs the +7 ppm /
  45 ms ramp today). The dual-tone method must be sanity-checked against the coded
  sweep for absolute offset, since it aliases past ±0.4 s.
