package sink

import (
	"math"
	"testing"

	"ondaire/internal/stream"
)

// These tests are a PURE, deterministic closed-loop simulation of the low-pass-filtered
// PROPORTIONAL servo (servo.go). No clock, no goroutine: the test IS the plant.
//
// SIGN / CONTROL LAW (the maths the sim verifies):
//
//   The servo's actuator is the resample RATIO: input samples consumed per output sample.
//   fedPTS = master time of the read cursor, advancing FrameSamples*ratio per step. The
//   sensor is the play-head phase error the ENGINE computes (servo never sees the queue):
//
//       phaseErr = fedPTS − L − shouldPTS          (L = constant configured device latency)
//
//   The servo drives target = 1 − Kp·lowpass(phaseErr), so phaseErr>0 (content AHEAD) ⇒
//   ratio<1 (consume input slower, fall back).
//
//   Plant model of a virtual DAC running dacPPM off nominal. The device WRITE BLOCKS, so
//   the DAC drain paces the loop: one frame of room frees every FrameNanos/(1+dacPPM/1e6)
//   of real time, so that is how far nowLocal (==shouldPTS) advances per step. The read
//   cursor advances FrameSamples*ratio of INPUT per step (→ fedPTS). Hence
//       d(phaseErr)/step = FrameNanos·[ratio − 1/(1+dacPPM/1e6)] ≈ FrameNanos·[ratio−1+dacPPM/1e6]
//   Lock ⇔ ratio = 1 − dacPPM/1e6, i.e. ratePPM → −dacPPM.
//
//   P-only (no integral) holds a STANDING phase error e_ss = dacPPM/(1e6·Kp): the loop
//   needs a non-zero error to command the rate that cancels the drift. That is expected
//   and acceptable (small, stable, absorbed by per-node delay calibration). The previous
//   PI integral existed to null it, but it wound up against the queue jitter into a
//   sawtooth — see TestServoNoSawtoothLongRun, the regression guard.

const nsPerSample = 1e9 / float64(stream.SampleRate)

// samplesToNs converts a (fractional) per-channel sample count to nanoseconds.
func samplesToNs(samples float64) int64 { return int64(samples * nsPerSample) }

// standingErrNs is the expected P-only standing phase error for a dacPPM drift at gain Kp.
func standingErrNs(dacPPM, kp float64) float64 { return dacPPM * 1e3 / kp }

// fastServoCfg converges quickly for tests: brisk gain, short filter, fast slew, so a few
// thousand 20 ms steps settle. Shared by servo_test and sink_test. Hotter than the
// production defaultServoConfig (gentler to ride out the Pi's ±10 ms snd_pcm_delay
// jitter); the noise-free sim can afford the speed.
func fastServoCfg() servoConfig {
	return servoConfig{
		Kp:       0.3,  // τ ≈ 3.3 s; 1 ms phase error → 300 ppm
		N:        4,    // short filter — sim is noise-free
		ClampPPM: 300,  // ±0.03% — same envelope as production
		SlewPPM:  4000, // ppm/s — fast so tests converge in a few hundred frames
	}
}

// lcg is a cheap deterministic ±1 source (no math/rand, reproducible).
func lcg(seed int64) func() float64 {
	s := seed
	return func() float64 {
		s = s*6364136223846793005 + 1442695040888963407
		return float64(uint64(s)>>40)/float64(uint64(1)<<24)*2 - 1
	}
}

// dacSim is the closed-loop plant: a virtual DAC at dacPPM whose blocking drain paces the
// loop. It runs the servo for `steps` frames, optionally injecting ±jitter samples of
// measurement noise on the phase error (the Pi's snd_pcm_delay frame-quantization). The
// anchor starts the play-head phase at ~0 — what the engine's Phase-B prime achieves
// before the servo takes over.
func dacSim(s *rateServo, dacPPM float64, steps int, jitterSamples float64) {
	const L = 0 // constant configured latency cancels in the dynamics; 0 ⇒ phaseErr starts at 0
	var nowLocal int64
	var consumed float64
	stepNs := int64(float64(stream.FrameNanos) / (1 + dacPPM/1e6)) // DAC-paced write return
	ratio := s.currentRatio()
	rnd := lcg(-0x61C8864680B583EB)
	for i := 0; i < steps; i++ {
		nowLocal += stepNs
		consumed += float64(stream.FrameSamples) * ratio
		fedPTS := int64(consumed * nsPerSample)
		phaseErr := fedPTS - L - nowLocal
		if jitterSamples > 0 {
			phaseErr += int64(rnd() * jitterSamples * nsPerSample)
		}
		ratio = s.observe(phaseErr, nowLocal, true)
	}
}

// TestServoConvergesFastDAC: a +150 ppm (fast) DAC drives the ratio to ≈ −150 ppm
// (SIGN: fast DAC ⇒ ratio<1) and the filtered phase error settles at the standing offset.
func TestServoConvergesFastDAC(t *testing.T) {
	cfg := fastServoCfg()
	s := newRateServo(cfg)
	dacSim(s, 150, 30000, 0)
	if math.Abs(s.ratePPM()-(-150)) > 5 {
		t.Fatalf("ratePPM=%.2f, want ≈ -150 (fast DAC ⇒ ratio<1)", s.ratePPM())
	}
	ess := standingErrNs(150, cfg.Kp)
	if got := float64(s.phaseErr()); math.Abs(got-ess) > 0.25*ess {
		t.Fatalf("phaseErr=%.0f ns, want ≈ +%.0f ns (P-only standing error)", got, ess)
	}
}

// TestServoConvergesSlowDAC: a −150 ppm (slow) DAC drives the ratio to ≈ +150 ppm and the
// standing error is negative.
func TestServoConvergesSlowDAC(t *testing.T) {
	cfg := fastServoCfg()
	s := newRateServo(cfg)
	dacSim(s, -150, 30000, 0)
	if math.Abs(s.ratePPM()-150) > 5 {
		t.Fatalf("ratePPM=%.2f, want ≈ +150 (slow DAC ⇒ ratio>1)", s.ratePPM())
	}
	ess := standingErrNs(-150, cfg.Kp)
	if got := float64(s.phaseErr()); math.Abs(got-ess) > 0.25*math.Abs(ess) {
		t.Fatalf("phaseErr=%.0f ns, want ≈ %.0f ns (P-only standing error)", got, ess)
	}
}

// TestServoGentleNeverRailsOnJitter: the PRODUCTION (default) config must track a real DAC
// drift while the Pi's ±10 ms (~±480 sample) snd_pcm_delay jitter is present WITHOUT
// railing at ±ClampPPM. The N-tap low-pass is the noise filter; the correction must stay
// well off the clamp.
func TestServoGentleNeverRailsOnJitter(t *testing.T) {
	cfg := defaultServoConfig()
	s := newRateServo(cfg)
	dacPPM := 40.0 // generous-but-realistic crystal offset; leaves clamp headroom for jitter
	var nowLocal int64
	var consumed float64
	stepNs := int64(float64(stream.FrameNanos) / (1 + dacPPM/1e6))
	ratio := 1.0
	rnd := lcg(99)
	var maxAbsCorr, sumPPM float64
	var nPPM int
	const steps = 120000 // ~40 min sim (instant maths)
	for i := 0; i < steps; i++ {
		nowLocal += stepNs
		consumed += float64(stream.FrameSamples) * ratio
		fedPTS := int64(consumed * nsPerSample)
		phaseErr := fedPTS - nowLocal + int64(rnd()*480*nsPerSample) // ±480-sample (±10 ms) jitter
		ratio = s.observe(phaseErr, nowLocal, true)
		if i > steps/2 { // past convergence: track the mean (lock) and worst instantaneous excursion
			sumPPM += s.ratePPM()
			nPPM++
			if a := math.Abs(s.ratePPM()); a > maxAbsCorr {
				maxAbsCorr = a
			}
		}
	}
	if meanPPM := sumPPM / float64(nPPM); math.Abs(meanPPM-(-40)) > 10 {
		t.Fatalf("gentle servo did not track +40 ppm DAC: mean ratePPM=%.1f, want ≈ -40", meanPPM)
	}
	if maxAbsCorr > 250 {
		t.Fatalf("gentle servo railed on jitter (max|ratePPM|=%.1f); must stay well off ±%.0f", maxAbsCorr, cfg.ClampPPM)
	}
}

// TestServoNoSawtoothLongRun is the REGRESSION GUARD for the bug this servo replaces. With
// the production config, a realistic small DAC drift, and the ±10 ms queue jitter, the
// low-frequency component of the phase error must NOT wander — i.e. no multi-minute
// wind-up sawtooth. We average the filtered phase error in 30 s blocks across the settled
// second half and assert every block mean stays within a tight band of the standing error.
// (The old PI integral produced ~±20 ms block-to-block swings here.)
func TestServoNoSawtoothLongRun(t *testing.T) {
	cfg := defaultServoConfig()
	s := newRateServo(cfg)
	dacPPM := 20.0 // realistic crystal offset
	var nowLocal int64
	var consumed float64
	stepNs := int64(float64(stream.FrameNanos) / (1 + dacPPM/1e6))
	ratio := 1.0
	rnd := lcg(1234567)
	const steps = 180000 // ~60 min sim
	const blk = 1500     // 30 s blocks
	var blockSum float64
	var blockN int
	var blockMin, blockMax = math.Inf(1), math.Inf(-1)
	var sumPPM float64
	var nPPM int
	for i := 0; i < steps; i++ {
		nowLocal += stepNs
		consumed += float64(stream.FrameSamples) * ratio
		fedPTS := int64(consumed * nsPerSample)
		phaseErr := fedPTS - nowLocal + int64(rnd()*480*nsPerSample)
		ratio = s.observe(phaseErr, nowLocal, true)
		if i >= steps/2 { // settled second half
			sumPPM += s.ratePPM()
			nPPM++
			blockSum += float64(s.phaseErr())
			blockN++
			if blockN == blk {
				m := blockSum / float64(blockN)
				if m < blockMin {
					blockMin = m
				}
				if m > blockMax {
					blockMax = m
				}
				blockSum, blockN = 0, 0
			}
		}
	}
	ess := standingErrNs(dacPPM, cfg.Kp) // ≈ 400 µs at 20 ppm, Kp 0.05
	// Block means must hug e_ss: a wandering/sawtoothing loop would blow this range open.
	if span := blockMax - blockMin; span > 1_000_000 { // 1 ms — vs the old ~20 ms sawtooth
		t.Fatalf("phase error wanders (block-mean span=%.0f ns over the settled half); "+
			"a stable P loop should hold ~e_ss=%.0f ns flat", span, ess)
	}
	if meanPPM := sumPPM / float64(nPPM); math.Abs(meanPPM-(-dacPPM)) > 5 {
		t.Fatalf("did not lock to %.0f ppm: mean ratePPM=%.1f", -dacPPM, meanPPM)
	}
}

// TestServoClampsOnAbsurdDrift: an impossible drift saturates the correction at the
// ±ClampPPM rail and never beyond.
func TestServoClampsOnAbsurdDrift(t *testing.T) {
	cfg := fastServoCfg()
	s := newRateServo(cfg)
	dacSim(s, 5000, 30000, 0) // 5000 ppm — far past the clamp
	if math.Abs(s.ratePPM()) > cfg.ClampPPM+0.001 {
		t.Fatalf("ratePPM=%.2f exceeds ±%.0f clamp", s.ratePPM(), cfg.ClampPPM)
	}
	if math.Abs(s.ratePPM()) < cfg.ClampPPM-1 {
		t.Fatalf("ratePPM=%.2f did not saturate near the ±%.0f clamp on absurd drift", s.ratePPM(), cfg.ClampPPM)
	}
}

// TestServoUnsyncedFreezes: while synced==false the servo holds the ratio and does NOT
// fold the sample into the filter — no offset to align against, so the loop must not move.
func TestServoUnsyncedFreezes(t *testing.T) {
	s := newRateServo(fastServoCfg())
	dacSim(s, 200, 8000, 0)
	locked := s.currentRatio()
	if math.Abs((locked-1)*1e6) < 50 {
		t.Fatalf("precondition: servo did not engage before freeze (ratePPM=%.1f)", s.ratePPM())
	}
	var nowLocal int64 = 1_000_000_000
	for i := 0; i < 5000; i++ {
		nowLocal += stream.FrameNanos
		got := s.observe(samplesToNs(50_000), nowLocal, false) // huge error, but unsynced
		if got != locked {
			t.Fatalf("step %d: unsynced changed ratio %.9f -> %.9f", i, locked, got)
		}
	}
}

// TestServoResetReturnsToUnity: reset clears the filter and phase, so the ratio is back at
// exactly 1 (a fresh session starts un-skewed).
func TestServoResetReturnsToUnity(t *testing.T) {
	s := newRateServo(fastServoCfg())
	dacSim(s, 200, 4000, 0)
	if s.currentRatio() == 1.0 {
		t.Fatal("precondition: servo never moved off unity")
	}
	s.reset()
	if s.currentRatio() != 1.0 {
		t.Fatalf("reset left ratio at %.9f, want 1.0", s.currentRatio())
	}
	if s.ratePPM() != 0 || s.phaseErr() != 0 {
		t.Fatalf("reset left telemetry dirty: ratePPM=%.3f phaseErr=%d", s.ratePPM(), s.phaseErr())
	}
}
