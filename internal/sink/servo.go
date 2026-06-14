package sink

// servoConfig holds the phase-lock servo parameters. The servo is a low-pass-filtered
// PROPORTIONAL controller: it takes the play-head PHASE ERROR (computed by the engine
// against the master clock — NOT the live device queue), low-passes it over the last N
// observations, and maps that filtered error DIRECTLY to the resampler RATIO. The
// blocking device write paces production; this loop only corrects the residual rate/phase.
//
// Why P, not PI. The plant is a pure integrator (ratio sets how fast the play head
// advances, so the phase error is the integral of the rate offset). A P controller on an
// integrator is a 1st-order, unconditionally-stable loop with time constant τ = 1/Kp and
// NO overshoot. The previous PI controller's integral wound up and saturated against the
// ±10 ms snd_pcm_delay frame-quantization that leaks into the phase error, producing a
// multi-minute limit-cycle SAWTOOTH on hardware (acoustically measured: ~20 ms peak-to-
// peak, never converging). Dropping the integral removes the wind-up entirely.
//
// The cost of P-only is a STANDING phase error e_ss = δ/Kp for a constant crystal drift δ
// (e.g. ~100 µs at 10 ppm with Kp = 0.1). That offset is small, STABLE, and absorbed by
// the per-node delay calibration (delayOffset / equalizeDelay); it drifts only slowly with
// temperature. Far better than the sawtooth.
//
// The low-pass is the jitter filter. The phase error carries the device queue's ±10 ms
// one-frame fill/drain quantization (it tracks deviceDelay ~1:1). An N-tap moving average
// rejects it (≈√N), so the ratio doesn't thrash drops/inserts on pure jitter. Because the
// loop τ (5–10 s) ≫ any sane filter length (N·20 ms), N is essentially a free jitter-
// rejection knob and does not threaten stability.
type servoConfig struct {
	Kp       float64 // proportional gain: ratio per second of phase error (units 1/s)
	N        int     // low-pass window: number of phase-error observations to average
	ClampPPM float64 // ratio clamp, ± (a real crystal is well under this)
	SlewPPM  float64 // max |Δratio| per second, ppm/s (safety: must NOT be the binding term)
}

func defaultServoConfig() servoConfig {
	// Kp 0.05 ⇒ τ ≈ 20 s; standing error ≈ 200 µs at 10 ppm crystal (static, absorbed by
	// per-node delay calibration). N 64 (≈1.3 s at the 20 ms frame cadence) gives ~8×
	// rejection of the ±10 ms queue jitter, keeping the proportional reaction well off the
	// clamp. Gain and filter are coupled: high Kp shrinks the standing error and slow
	// thermal residual but amplifies jitter, so N must grow with Kp. Because τ ≫ the filter
	// length the loop stays well-damped. Clamp/slew are loose safety rails, not control
	// terms. Validate/retune in the offline sim (servo_test.go).
	return servoConfig{
		Kp:       0.05,
		N:        64,
		ClampPPM: 300, // ±0.03% — covers any real crystal; inaudible
		SlewPPM:  300, // loose enough not to bind for real (tens-of-ppm) corrections
	}
}

// rateServo turns play-head phase-error observations into a resampler ratio (≈1). One per
// session; reset on generation change. Pure: no goroutine/clock/locking (the Playout
// mutex guards it).
type rateServo struct {
	cfg        servoConfig
	have       bool      // first observation seen (clock seeded)
	lastT      int64     // local ns of the previous observe (for dt)
	buf        []float64 // ring buffer of the last N phase errors, seconds
	bufSum     float64   // running sum of buf (filter is bufSum/bufLen)
	bufLen     int       // valid samples in buf (< N during fill)
	bufPos     int       // next write index
	ratio      float64   // current input/output ratio (≈1)
	phaseErrNs float64   // last FILTERED phase error (telemetry)
}

func newRateServo(cfg servoConfig) *rateServo {
	if cfg.Kp <= 0 {
		cfg = defaultServoConfig()
	}
	if cfg.N < 1 {
		cfg.N = defaultServoConfig().N
	}
	return &rateServo{cfg: cfg, ratio: 1, buf: make([]float64, cfg.N)}
}

// pushFilter folds one raw error (seconds) into the moving average and returns the new
// filtered value.
func (s *rateServo) pushFilter(e float64) float64 {
	if s.bufLen == s.cfg.N {
		s.bufSum -= s.buf[s.bufPos]
	} else {
		s.bufLen++
	}
	s.buf[s.bufPos] = e
	s.bufSum += e
	s.bufPos++
	if s.bufPos == s.cfg.N {
		s.bufPos = 0
	}
	return s.bufSum / float64(s.bufLen)
}

// observe folds one play-head phase-error measurement and returns the updated ratio.
//
//	phaseErrNs : (fedPTS − configuredDeviceLatency) − shouldPTS, ns. >0 ⇒ play head AHEAD
//	             ⇒ too fast ⇒ ratio<1 (consume input slower). Computed by the engine; the
//	             servo never sees the live device queue.
//	nowLocal   : monotonic local ns (for dt)
//	synced     : clock-sync valid; if false, hold the ratio and skip the filter (do not
//	             fold a stale/unsynced sample, and do not advance across a gap)
func (s *rateServo) observe(phaseErrNs, nowLocal int64, synced bool) float64 {
	if !s.have {
		s.have = true
		s.lastT = nowLocal
		return s.ratio
	}
	dtNs := nowLocal - s.lastT
	s.lastT = nowLocal
	if dtNs <= 0 || !synced {
		return s.ratio // hold; do not filter across a gap or while unsynced
	}
	dtSec := float64(dtNs) / 1e9
	eFilt := s.pushFilter(float64(phaseErrNs) / 1e9)
	s.phaseErrNs = eFilt * 1e9

	corr := s.cfg.Kp * eFilt // proportional map: filtered phase error → ratio offset
	clampRatio := s.cfg.ClampPPM * 1e-6
	if corr > clampRatio {
		corr = clampRatio
	} else if corr < -clampRatio {
		corr = -clampRatio
	}
	target := 1.0 - corr // minus: phaseErr>0 (ahead) ⇒ ratio<1 (consume input slower)

	maxStep := s.cfg.SlewPPM * 1e-6 * dtSec
	if d := target - s.ratio; d > maxStep {
		s.ratio += maxStep
	} else if d < -maxStep {
		s.ratio -= maxStep
	} else {
		s.ratio = target
	}
	return s.ratio
}

// currentRatio returns the last ratio without folding a new observation (used between
// observes, e.g. during priming).
func (s *rateServo) currentRatio() float64 { return s.ratio }

// ratePPM returns the rate correction in ppm (for SinkStats.RatePPM).
func (s *rateServo) ratePPM() float64 { return (s.ratio - 1) * 1e6 }

// phaseErr returns the last FILTERED phase error in ns (for SinkStats.PhaseErrNs). It is
// the error the servo actually acts on (queue jitter low-passed out), not the raw sample.
func (s *rateServo) phaseErr() int64 { return int64(s.phaseErrNs) }

// reset clears state for a new session.
func (s *rateServo) reset() {
	s.have = false
	s.lastT = 0
	s.ratio = 1
	s.phaseErrNs = 0
	s.bufSum = 0
	s.bufLen = 0
	s.bufPos = 0
	for i := range s.buf {
		s.buf[i] = 0
	}
}
