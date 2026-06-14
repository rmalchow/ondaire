# Playout pipeline & phase-lock control loop

Authoritative description of how decoded audio reaches the speaker in phase with
the master clock, end to end. This is the mechanism for the sink
(`internal/sink/*` + `internal/sink/device/*`).

The doc has two registers. **§A Spec** is the compressed mechanism — formulas,
constants, structures — for an engineer who knows the codebase. **§B How it works**
is the same mechanism in plain language, with the *why*, for someone new. Read
whichever you need; they describe one and the same system.

---

# A. Spec

## A.1 Pipeline

```
network receiver ─► decode ─► jitter buffer ─► resampler ─► gain ─► device port
(UDP+FEC | TCP)   (PCM noop  (seq-keyed       (pull 4-tap (live    (Sink.Write
                   | Opus)    reorder,         Catmull-Rom) volume,  BLOCKS = pacer)
                              PTS-stamped)                  1-frame
                                                            ramp)
```

- Canonical frame (`internal/stream/wire.go`): `SampleRate=48000`, `Channels=2`,
  `BytesPerSmpl=2` (s16le), `FrameSamples=960`, `FrameBytes=3840`, `FrameDuration=20`
  ms, `FrameNanos=20_000_000`.
- Intake → sink seam: the receiver/decoder calls `contracts.Sink.Push(gen, seq, pts,
  payload)` with canonical PCM (opus decoded first; `cmd/ensemble/main.go` deliver
  loop). `Push` is fire-and-forget, non-blocking; copies payload; drops+counts
  stale-gen / late frames; signals the scheduler.
- `pts` is **master time** (set at the source). A frame's content should hit the
  speaker at `MasterToLocal(pts + bufferMs − delayOffsetMs + equalizeDelay)`.

## A.2 Jitter buffer (`jitter.go`)

Bounded seq-keyed reorder map (`map[uint64]*slot`, cap `defaultCapacity=256` ≈ 5.1
s). Not goroutine-safe; the `Playout` mutex guards it. `slot{pts, payload}` owns a
copied payload. `setOrigin` fixes `nextSeq` on the first session frame. `insert`
rejects `seq < nextSeq` (late) and, when full, evicts the furthest-future slot only
if the new seq is nearer; duplicate seq overwrites idempotently (FEC double-delivery).
`hasPending() = hasNext && hasMax && nextSeq <= maxSeq` (drained-to-end test).
Absorbs **network** jitter and holds the bulk of `bufferMs`.

## A.3 Resampler (`resampler.go`) — pull, 4-tap Catmull-Rom

Per channel, interleaved L/R. `feed(frame)` appends one `FrameBytes` frame (first
feed seeds `leadPad=3` lookback = the first sample held). `process(ratio)` emits
**exactly `FrameSamples`** output; the rate correction is entirely in `ratio`
(input samples advanced per output sample, ≈1) — output length never varies.

Catmull-Rom for fractional `t∈[0,1)` between `p1,p2` (neighbours `p0,p3`):

```
y(t) = 0.5*( 2*p1 + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t² + (-p0+3*p1-3*p2+p3)*t³ )
```

Output sample `k` reads input position `pos + k*ratio`; `idx=int(p)`, `t=p-idx`,
taps `atIdx(idx-1..idx+2)` (clamped to buffer ends). Per call: `adv = FrameSamples*ratio`,
`consumed += adv`, `pos += adv`; surplus/deficit vs `FrameSamples` accrues to
`dropped`/`injected` (realized rate match, per-channel samples). Then drop whole
consumed samples from the front, keeping `leadPad` lookback (`pos` stays ≥ leadPad;
buffer bounded). Constants: `leadPad=3`, `lookahead=2`, `needInput = FrameSamples +
8 + lookahead + 1`. `consumedSamples()` is the per-session cursor (the play-head
reference); `reset()` zeroes it; lifetime inject/drop survive reset.

ratio=1 ⇒ integer cursor ⇒ Catmull-Rom returns the sample exactly (bit-identical
passthrough). ratio>1 ⇒ compress (consume input faster, catching up); ratio<1 ⇒
stretch.

## A.4 Gain (`gain.go`)

Last stage before the device. Target gain stored as `atomic.Uint64`
(`math.Float64bits`); `SetGain` is lock-free from any goroutine. `apply` ramps
`current → target` linearly across the frame's `FrameSamples` sample-times (both
channels share a factor), rounds, clamps int16. Settles in one 20 ms frame, no
zipper. Unity (`current==target==1.0`) short-circuits to bit-identical passthrough.

## A.5 Clock (`internal/clock`)

`Follower` implements `contracts.Clock`: `MasterNow`, `MasterToLocal`,
`LocalToMaster`, all gated on `est.offset()` (`ok=false` until confident → playout
must not start). All local-time values use `clock.MonoNow()` (monotonic), never wall
time. `monotoNow()` in `sink.go` is that same clock. See [clock sync](clock-sync.md).

## A.6 The control loop (`sink.go`) — DAC-pull phase lock

The device's **blocking `Write` is the rate pacer** — it returns at the DAC's true
drain rate. There is **no master-deadline sleep in steady state**.

**Per-slot loop** (`loop()`), under-mutex feed+render, unlocked write,
under-mutex observe:

```
mu.Lock
  feedLocked()                          // top up resampler to >= needInput
  out := rs.process(servo.currentRatio())
  gain.apply(out)
mu.Unlock
writeStartNs = now(); out.Write(out); writeStartNs = 0   // BLOCKS = pacer
mu.Lock
  observeLocked()                       // fold one phase measurement → servo
mu.Unlock
```

**Phase error** (engine-computed in `observeLocked`):

```
fedPTS    = fedAnchorPTS + (consumedSamples − fedAnchorCon)·1e9/SampleRate
shouldPTS = LocalToMaster(now) − bufferNs + delayOffsetNs − equalizeDelayNs
phaseErr  = fedPTS − deviceLatencyNs − shouldPTS         // >0 ⇒ play head AHEAD
servo.observe(phaseErr, now, synced)
```

- `fedPTS` = master time of the resampler read cursor, anchored at each prime
  (relative to consumed-at-prime — see A.8).
- `deviceLatencyNs` = the backend's **CONSTANT** configured latency
  (`LatencyReporter.ConfiguredLatencyNs()`), a fixed offset — **NOT** the live queue.
- The live device queue (`snd_pcm_delay` via `DelayReporter.Delay()`) is **telemetry
  only** → `SinkStats.DeviceDelayNs`. It is never a control input.

Regulating the cursor against the master clock makes the plant a **pure integrator
with no dead time** → a low-pass-filtered P controller is stable (1st-order, no overshoot).

## A.7 The servo (`servo.go`) — low-pass-filtered P on the resample ratio

Actuator = resample `ratio` (`ratePPM = (ratio−1)·1e6`).

```
e_filt = movingAvg_N(phaseErr/1e9)            // seconds; N-tap low-pass of the queue jitter
corr   = Kp·e_filt;  |corr| ≤ ClampPPM·1e-6   // direct proportional map (no integral)
target = 1 − corr                             // minus: ahead ⇒ ratio<1 ⇒ consume slower
ratio += clamp(target − ratio, ∓SlewPPM·1e-6·dt)   // slew-limited (safety only)
```

Plant is an integrator ⇒ a P controller is a **1st-order, unconditionally-stable**
loop, time constant **τ = 1/Kp**, no overshoot. There is **no integral**: an earlier
PI design wound its integral up against the ±10 ms `snd_pcm_delay` frame-quantization
that leaks into `phaseErr` (it tracks `deviceDelay` ~1:1), producing a multi-minute
limit-cycle **sawtooth** on hardware (acoustically ~20 ms peak-to-peak, never
converging). Dropping the integral removes the wind-up; the **N-tap low-pass** rejects
the queue jitter (≈√N) so the proportional term doesn't thrash drops/inserts on noise.

Cost of P-only: a **standing** phase error `e_ss = δ/Kp` for a constant crystal drift δ
(≈100 µs at 10 ppm with Kp 0.1; ≈200 µs at Kp 0.05). It is small, **stable**, and
absorbed by the per-node delay calibration (delayOffset/equalizeDelay); it drifts only
slowly with temperature. Far better than the sawtooth.

`defaultServoConfig()` (Kp 0.05, N 64) is **tuned in the offline sim (`servo_test.go`)**;
`TestServoNoSawtoothLongRun` is the regression guard. Gain and filter are coupled — high
Kp shrinks the standing error and thermal residual but amplifies jitter, so N grows with
Kp; because τ (≈20 s) ≫ the filter length the loop stays well-damped. Clamp ±300 ppm
(covers any real crystal, inaudible); slew deliberately loose so it never binds. While
`synced=false` or `dt≤0` the servo holds the ratio and does not fold the sample into the
filter. Authority is tiny by design: the **prime lands the phase**; the servo only holds
it and tracks slow crystal drift.

## A.8 Prime (`prime()`, per session, once)

1. Skip overdue frames (`deadlineLocal(nextSeq) < now` → `LateDrop++`, advance).
2. Sleep until `deadline − deviceLatency`.
3. Pre-fill the device with silence: `nPrefill = max(2, deviceLatency/FrameNanos)`
   silent `Write`s (also opens a lazily-opened resilient device; refreshes
   `deviceLatencyNs` from `LatencyReporter` once open).
4. Hand over: `fedAnchorPTS = slotPTS(nextSeq)`, `fedAnchorCon = consumedSamples()`,
   `dacPrimed = true` → initial `phaseErr ≈ 0`.

**Relative anchor:** `fedPTS` is measured relative to `consumedSamples` *at prime*, so
a re-anchor (equalize/delay change, re-arm via `SetDelayOffset`/`SetEqualizeDelay`)
cannot inject a phantom phase offset from input consumed before it. Both
`SetDelayOffset` (per-node output-delay calibration) and `SetEqualizeDelay`
(cross-room equalization) drop the buffer, clear the origin, fire the `RestartFunc`,
and re-prime under the new anchor; `SetEqualizeDelay` is a no-op on an unchanged value
(avoids re-anchor glitches from the master's 1 Hz re-assert).

## A.9 Buffer topology — two buffers for two jitters

| buffer | size | absorbs |
|---|---|---|
| INPUT jitter buffer (`jitter.go`) | bulk of `bufferMs` | NETWORK jitter (bursty Wi-Fi) |
| OUTPUT device buffer (alsa) | ~40 ms / 2 frames (`ENSEMBLE_ALSA_LATENCY_MS`, 20..500) | scheduling slack only |

The output buffer cannot absorb network jitter (it is downstream of the resampler) —
it is only scheduling slack for the playout goroutine plus something for the blocking
`Write` to block on. Because the loop folds `deviceLatency` in as a **constant**, the
output buffer size does not affect loop stability. `DefaultBufferMs=300` must exceed
the device's configured latency; the jitter window = `bufferMs − deviceLatency`.

## A.10 Device port + adapters (`internal/sink/device`)

`device.Sink{ Write, Close }` is the only mandatory contract: **`Write` MUST block**
until the device can accept the next frame (the pacer). Optional capabilities,
type-asserted by `device.Query[T]`, forwarded by the resilient wrapper via
`As(any) bool`: `DelayReporter` (phase probe / telemetry), `LatencyReporter`
(constant offset + prime depth), `Flusher`, `Interrupter`, `StatsReporter`,
`DeviceSelector`, `Reviver`, `ActiveReporter`.

| adapter | pacer | phase probe (`Delay`) | latency | notes |
|---|---|---|---|---|
| alsa | `snd_pcm_writei` blocks | yes (`snd_pcm_delay`, telemetry) | ~40 ms | unlocked writei; `Interrupt`/`Flush` = `snd_pcm_drop`; xrun→`recover` |
| exec | pipe backpressure | no | — | player subprocess; `Flush`/respawn; opaque latency → ratio≈1 |
| file | internal real-time clock | no | — | drift-free deadline accumulator; PCM tee |
| null | internal real-time clock | no | — | drift-free accumulator; discard |

The **resilient** wrapper (`resilient.go`, the default output) is a self-healing
failover chain: opens candidates by name through the registry, rotates on write
failure, rests with exponential backoff after `resilientMaxSweeps=3` full failed
sweeps; forwards capabilities to the live candidate; overlays wrapper-only stats
(`Kind`, `Rotations`, `Resting`, `BackoffMs`). The blocking `Write` runs **unlocked**
so `Interrupt` reaches the candidate mid-write.

## A.11 Robustness

- **Starvation watchdog** (`checkStarvationLocked`, 2 s `defaultWatchdog`): first
  interval of silence → fire `RestartFunc` once (the subscriber issues a wire
  RESTART), stay armed; second interval still silent → disarm (reset servo/rs/jb,
  flush).
- **Write-stall guard** (`watchWrites`): a `Write` parked > `3·deviceLatencyNs`
  (floor 500 ms) → `Interrupt` so the loop unparks and the watchdog can fire.
- **Input-starved underrun** (`feedLocked`): jitter buffer drains mid-top-up → pad
  silence, count as `Silence`, log rate-limited (1/s). Audible gap made diagnosable.

## A.12 Telemetry (`contracts.SinkStats`)

`Played`, `Silence`, `LateDrop`, `StaleGen`, `Synced`, `RatePPM` (`(ratio−1)·1e6`),
`Buffered` (jb depth), `DeviceDelayNs` (live queue, telemetry), `PhaseErrNs` (≈0 in
lock), `Calibrated` (synced && phase probe present), `SamplesInjected/Dropped`
(realized resample corrections). **Cross-room equalization** reads
`DeviceDelayNs − PhaseErrNs` from STATUS as the stable per-room device-queue depth
(the master delays faster rooms to match the slowest via `SetEqualizeDelay`).

---

# B. How it works

## B.1 The problem: two clocks that disagree

Every speaker in a room group must play the same instant of audio at the same wall
moment, or you hear an echo/flam. But each playback node has its own quartz crystal
driving its DAC, and no two crystals run at exactly the same speed — they differ by
tens of parts-per-million. Left alone, two speakers started in perfect sync drift
apart by tens of milliseconds over a few minutes. On top of that the audio arrives
over Wi-Fi, which is bursty and lossy: packets clump up, arrive out of order, or go
missing entirely.

So there are really two separate things to get right, and it helps to keep them
apart in your head:

- **Phase** — *when* a given chunk of audio reaches the speaker. This is the
  cross-room alignment. We want chunk X to come out of every speaker at the same
  master-clock instant.
- **Rate** — *how fast* audio is fed to the DAC. If we feed slightly faster or
  slower than the crystal actually drains, the device's internal buffer slowly fills
  or empties until it overflows (a click) or underflows (a dropout).

The whole pipeline is built around solving phase and rate without letting them fight
each other.

## B.2 The conveyor belt: how a frame travels

Think of the pipeline as a short conveyor belt. Audio arrives from the network in
20-millisecond chunks called *frames* (exactly 960 stereo samples, 3840 bytes). If
the audio is compressed (Opus), it's decoded back to raw PCM first. Then each frame,
tagged with a sequence number and a master-clock timestamp (its *PTS*), is handed to
the sink by calling `Push`. `Push` never blocks and never waits — it just copies the
frame in and pokes the playout goroutine. If a frame is hopelessly late or belongs to
an old session, it's dropped and counted right there.

From `Push` the frame lands in the **jitter buffer**. This is the first of two
buffers, and its job is to absorb *network* messiness. Frames are stored keyed by
sequence number, so if packet 7 arrives before packet 6, the buffer quietly holds 7
until 6 shows up and plays them in order. It's bounded (about five seconds' worth) so
a wedged stream can't grow it forever. Most of the configured buffer budget
(`bufferMs`, default 300 ms) lives here — that headroom is what lets a Wi-Fi burst
hiccup without you hearing it.

When it's time to play, frames are pulled out in sequence order and fed into the
**resampler**. After the resampler comes the **gain** stage (volume), and finally the
frame is written to the **device** — the actual speaker output. That last write is
the secret to the whole timing scheme, so let's build up to it.

## B.3 The pacer: let the speaker set the clock

Here's the key design choice. We do *not* have a metronome goroutine that wakes up
every 20 ms and shoves a frame at the device. Instead, the device's `Write` call is
defined to **block** until the device is ready for the next frame. A real sound card
only accepts a new frame when its buffer has drained enough room — and it drains at
the crystal's true speed. So when our loop calls `Write` and that call returns, that
return *is* the tick of the DAC's real clock. We call `Write` again immediately, it
blocks again, and the loop naturally runs at exactly the speed the hardware consumes
audio. This is "DAC-pull": the DAC pulls audio through the pipeline at its own pace.

This is elegant because it solves the *rate* problem for free, almost. The device can
never overflow or underflow from us feeding too fast or too slow, because we
physically can't feed faster than it drains — `Write` won't let us. Every output
device honours this same contract: the ALSA card blocks naturally; a pipe to a player
program blocks when the pipe fills; the file and null "devices" have no hardware to
push back, so they fake it with an internal real-time clock that sleeps exactly one
frame-period per write. From the engine's point of view they're all identical: call
`Write`, get paced.

## B.4 But the speaker's clock is the wrong clock

DAC-pull keeps the local device happy, but it doesn't keep two *different* speakers in
sync — each is now marching to its own crystal, and the crystals disagree. We've
solved rate locally but not phase across the room. We need to gently steer each node
so its audio comes out aligned to the shared **master clock**, not just to its own
crystal.

We can't change how fast the DAC drains — that's physics. The one knob we *do* have is
the **resampler's ratio**. The resampler reads input samples and produces output
samples using smooth 4-point (Catmull-Rom) interpolation. It always emits exactly one
frame of output per `Write` (it has to — the device demands fixed-size writes), but it
can choose how much *input* to consume to make that output. At ratio 1.0 it consumes
exactly one input frame and the output is bit-for-bit the input. At ratio 1.0001 it
consumes a hair more input per output frame — effectively playing the content very
slightly fast (time-compressing) so our play position creeps forward. At ratio 0.9999
it consumes a hair less — playing slightly slow, letting our position fall back. The
adjustment is far too small to hear (tens of ppm, like the crystal error it cancels),
but over seconds it's exactly enough to hold us on the master clock.

So: the blocking `Write` sets the rate; the resampler ratio nudges the phase. They
don't fight, because they act on different things.

## B.5 Measuring how wrong we are: the phase error

To steer, we need to know our error. After each frame is written, the engine computes
one number: how far ahead or behind the master clock our play position is.

We track a "play head" — the master-time position of the resampler's read cursor,
called `fedPTS`. Every output sample the cursor advances past corresponds to
1/48000th of a second of master time, so `fedPTS` is just the anchor we set at startup
plus (samples consumed since then) converted to nanoseconds. Then we ask the master
clock where playback *should* be right now — `shouldPTS` — which is the current master
time minus the buffer lead, adjusted by the per-node calibration offsets. The
difference, after subtracting a fixed allowance for how much audio sits in the
device's own buffer, is the phase error:

```
phaseErr = fedPTS − deviceLatency − shouldPTS
```

A positive error means our play head is *ahead* of where the master says it should be,
so we should slow down (ratio below 1). Negative means we're behind, so speed up.

There's a subtle but important point here about that `deviceLatency` term. A sound
card can actually tell you, live, how much audio is currently queued in its buffer
(`snd_pcm_delay`). It would be tempting to subtract *that* live number. **We
deliberately do not.** That live queue reading jitters by ±10 ms on a Raspberry Pi and
introduces a delay between cause and effect (dead time) that makes the control loop
hard to stabilise. Instead we subtract the device's *configured, constant* latency —
a fixed number that never moves. The live queue reading is still collected, but only
as **telemetry** (you can see it in the stats, and the master uses it for cross-room
equalization), never as a control input.

Why does using a constant make life so much easier? Because it turns the system into a
clean "pure integrator": the only thing that moves the phase error is the rate
difference, and the phase error is just the running sum of that difference over time.
There's no hidden dead time. There IS one residual noise term, though: because the
phase error compares the *fed* position against the wall clock at the moment the
blocking write returns, the device queue's one-frame (20 ms) fill/drain quantization
rides on the signal — the measured `phaseErr` tracks the live `deviceDelay` to within
~±10 ms. The servo has to filter that out (see below).

## B.6 Steering: the low-pass-filtered P servo

The servo is a **proportional** controller with a **low-pass filter** on its input.
Each measurement it averages the last N phase-error samples and maps that filtered
error directly to the ratio: a bigger (smoothed) error, a bigger nudge.

Why no integral? An integral drives a persistent offset to zero by accumulating it —
but the ±10 ms queue jitter on the phase error means the accumulator never sees a
clean signal, and an earlier PI version wound it up against that jitter into a
**sawtooth**: the rate ramped, snapped back, and repeated every few minutes, so the
two speakers swung ~20 ms apart and never settled (caught acoustically on a 1-hour
calibration run, invisible on short ones). Removing the integral removes the wind-up
entirely. The low-pass is the jitter filter: averaging N samples cuts the queue noise
by about √N, so the proportional term doesn't react to every twitch and thrash the
resampler with needless drops/inserts.

The cost is a small **standing error**: with only a proportional term, holding a
constant crystal drift δ needs a constant ratio correction, which needs a constant
(non-zero) phase error `e_ss = δ/Kp` to command it — roughly 100–200 µs for a typical
crystal at the default gain. That offset is small, *stable*, and the per-node delay
calibration absorbs it; it moves only as slowly as the crystal drifts with temperature.
A stable sub-millisecond offset is vastly better than a wandering 20 ms sawtooth.

The output is clamped to ±300 ppm (no real crystal drifts more than that, and even the
full clamp is inaudible) and slew-limited so the rate can never jump abruptly — both
are safety rails, not active control terms. Gain and filter length are coupled: a
higher gain shrinks the standing error and the slow thermal residual but amplifies the
jitter, so the filter has to grow with the gain. Because the loop is slow (time
constant ~20 s) and the filter is short (~1 s) by comparison, it stays well-damped.
The numbers were tuned in an offline simulation (`servo_test.go`) that models a
drifting DAC plus the ±10 ms jitter, with a long-run test that fails if the old
sawtooth ever reappears — treat them as sim-tuned, not sacred.

The philosophy is "set it once, then just hold." The servo's authority is tiny on
purpose. The heavy lifting of getting phase right is done once, at startup, by
priming. The servo's only ongoing job is to hold that lock and quietly track the slow
crystal drift.

## B.7 Priming: landing the phase before the music starts

When a session begins, we don't just start blasting frames and let the servo chase a
huge error into alignment — that would be audible. Instead we *prime*:

1. Throw away any frames whose moment has already passed (starting on a stale frame
   would force the servo to chase a big negative error).
2. Sleep until just before the first real frame is due — specifically until
   `deadline − deviceLatency`, so that after the frame sits in the device buffer for
   its latency, it emerges at exactly the right master-clock instant.
3. Pre-fill the device buffer with a couple of frames of silence, so there's
   something draining while we get going (and, for a lazily-opened device, this is
   what actually opens it).
4. Capture the anchor: record the play head's master time and the resampler's
   consumed-sample count *right now*. Because the first real frame is timed to its
   deadline, the initial phase error is essentially zero. The servo wakes up already
   locked and just has to keep it there.

That anchor — recording the consumed count *at the moment of priming* rather than from
session start — fixes a real bug. If you re-prime mid-session (say the user changes
the delay calibration, or the room is re-equalized), measuring the play head relative
to "now" means the samples consumed *before* the re-prime can't leak in as a phantom
phase jump. Re-anchoring is clean every time.

## B.8 Two buffers for two kinds of jitter

It's worth being explicit about why there are *two* buffers, because conflating them
is a common mistake. The **input jitter buffer** soaks up *network* jitter — the
bursty, lossy nature of Wi-Fi — and that's where almost all of the buffer budget
lives. The **output device buffer** (about 40 ms on ALSA, configurable via
`ENSEMBLE_ALSA_LATENCY_MS`) is something else entirely: it's just a bit of scheduling
slack so our playout goroutine has a small cushion if the OS is briefly late waking
it, plus a little something for the blocking `Write` to block against. It sits
*downstream* of the resampler, so it physically cannot help with network jitter — a
late packet is already a problem before audio ever reaches the device.

A nice consequence of folding device latency in as a *constant* (B.5) is that the size
of this output buffer doesn't affect the control loop's stability at all. You can make
it bigger on a flaky host to avoid underruns without retuning anything.

## B.9 When devices differ: the adapters

Not every output is a sound card. The pipeline talks to an abstract "device port" with
one rule — `Write` must block to pace — and everything beyond that is an optional
capability a given device may or may not offer. A real ALSA card offers a phase probe
(it can report its queue depth) and a known latency; a pipe to an external player
program offers neither (the player and its own buffers are opaque), so for those the
servo simply holds the ratio at 1 and trusts the prime alignment plus the per-node
calibration. The file and null sinks fake their pacing with a precise internal clock.

Wrapping all of this is the **resilient** backend, which is what actually runs by
default. It's a self-healing chain: it tries each available output in turn, and if the
live one fails mid-stream it rotates to the next; if everything's broken it backs off
and rests, retrying periodically rather than thrashing. The clever part is that
capability queries (like "do you have a phase probe?") transparently reach whichever
device is currently live, so the servo always sees the truth about the real hardware
playing right now, with no special-casing in the engine.

## B.10 When things go wrong

Three guards keep a bad situation from becoming a silent mystery:

- If frames stop arriving for two seconds, the **starvation watchdog** fires a
  RESTART request (asking the source to re-prime us) and stays armed; if another two
  seconds pass with nothing, it gives up cleanly and disarms so the group can
  self-heal.
- If a `Write` parks far longer than the device buffer could justify — a wedged
  device — the **write-stall guard** interrupts it so the loop can unstick and the
  watchdog can act, instead of hanging forever invisibly.
- If the jitter buffer runs dry mid-frame (a network gap longer than our buffer), we
  pad with silence rather than block, count it, and log it once a second — so a choppy
  stream shows up in the logs instead of being an unexplained glitch.

## B.11 What you can see: telemetry

Everything important is observable in `SinkStats`: frames `Played`, `Silence` inserted
for gaps, `LateDrop`/`StaleGen` discards, whether the clock is `Synced`, the current
rate correction `RatePPM` (≈ the crystal error once locked), the live `PhaseErrNs`
(≈ 0 when locked), the jitter `Buffered` depth, the live device queue `DeviceDelayNs`
(telemetry), `Calibrated`, and the actual sample-level corrections the resampler made
(`SamplesInjected`/`SamplesDropped`). The master uses one derived quantity,
`DeviceDelayNs − PhaseErrNs`, as a stable measure of each room's device-buffer depth,
and gently delays the faster rooms so every room emits the same instant together —
cross-room equalization built on the same numbers the local loop already produces.
