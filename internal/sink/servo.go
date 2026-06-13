package sink

import "ensemble/internal/stream"

// servoConfig holds the rate-servo parameters. The servo holds the output DEVICE
// QUEUE at a calibrated setpoint with a proportional controller: that is the only
// software-observable signal for DAC crystal drift (the playout scheduler is
// master-clock-locked, so consumed-vs-master measures the scheduler, not the DAC —
// the queue depth is where the DAC's true rate shows up). Kq is GENTLE (D64): the
// Pi's snd_pcm_delay swings ±10 ms, and the old Kq=1.5 mapped that to >300 ppm, so a
// single jitter sample railed the loop into the ±ClampPPM hunting that was the
// audible "stumbling". A gentle gain maps ±10 ms to tens of ppm — corrections stay
// small and smooth.
type servoConfig struct {
	WarmUp   int64   // ns to wait before calibrating (queue fill + EMA settle)
	QueueTau int64   // device-queue smoothing time constant, ns
	Kq       float64 // proportional gain: ppm per sample of queue error
	Ki       float64 // integral gain: ppm per (sample·second) of accumulated error
	ClampPPM float64 // output clamp, ± (a real crystal is well under this)
	SlewPPM  float64 // max |Δoutput| per second, ppm/s
}

func defaultServoConfig() servoConfig {
	return servoConfig{
		WarmUp:   3_000_000_000, // 3 s: let the device queue fill and the EMA settle
		QueueTau: 2_000_000_000, // 2 s: smooths the Pi's ±10ms snd_pcm_delay jitter
		Kq:       0.08,          // GENTLE proportional: 480 samples (10 ms) → ~38 ppm
		Ki:       0.01,          // integral: parks queue error at 0 (no droop / ramp).
		//                          Now valid because outLen actually moves the queue
		//                          (PLAN-playout-rate-lock); a real crystal is a STEP
		//                          disturbance the PI nulls — convergence in ~tens of s.
		ClampPPM: 300, // ±0.03% — covers any real crystal; inaudible
		SlewPPM:  20,  // ppm/s — gentle, no audible rate steps
	}
}

// rateServo turns a stream of (consumed, master, deviceDelay) observations into a
// playback-rate correction in ppm. One per session; reset on generation change.
// Pure: no goroutine, no clock, no locking (the Playout mutex guards it). Without a
// DelayReporter backend (deviceDelay ok=false) it returns 0 — DAC drift is then
// unobservable and accepted (the player owns its own clock).
type rateServo struct {
	cfg        servoConfig
	have       bool    // first observation seen (EMA + clock seeded)
	startMast  int64   // master ns of the first observation (warmup origin)
	lastMast   int64   // master ns of the previous observation (per-step dt)
	ddEMA      float64 // smoothed device-queue depth, samples (fast)
	ddRef      float64 // slow reference EMA (settle detection)
	calibrated bool    // setpoint captured (warmup elapsed)
	setpoint   float64 // target device-queue depth, samples
	integral   float64 // ∫queueErr dt (sample·seconds), anti-windup clamped
	outPPM     float64 // current correction, ppm (slew-limited, clamped)

	// telemetry (read by the sink's 1 Hz stats line)
	queueErr float64 // ddEMA − setpoint, samples
}

func newRateServo(cfg servoConfig) *rateServo {
	if cfg.QueueTau <= 0 {
		cfg = defaultServoConfig()
	}
	return &rateServo{cfg: cfg}
}

// observe folds one measurement and returns the updated correction in ppm.
//
//	consumedSamples : cumulative samples/ch written to the backend (unused now,
//	                  kept for the call signature + future backpressure mode).
//	masterNanos     : current master-clock time (ns).
//	deviceDelayNs,ok: queued audio between Write and speaker if the backend is a
//	                  DelayReporter; ok=false => no drift signal, return 0.
//
// A positive return means "produce samples faster" (resample ratio > 1).
func (s *rateServo) observe(consumedSamples, masterNanos, deviceDelayNs int64, ok bool) float64 {
	if !ok {
		// No queue to measure: the scheduler is master-locked, so there is no
		// software-observable DAC drift. Leave the rate at nominal.
		return s.outPPM
	}
	dd := float64(deviceDelayNs) * float64(stream.SampleRate) / 1e9

	if !s.have {
		s.have = true
		s.ddEMA = dd
		s.ddRef = dd
		s.startMast = masterNanos
		s.lastMast = masterNanos
		return s.outPPM // 0
	}

	dtNs := masterNanos - s.lastMast
	s.lastMast = masterNanos
	if dtNs <= 0 {
		return s.outPPM
	}

	// Smooth the device-queue depth (EMA, time-constant QueueTau).
	alpha := float64(dtNs) / float64(s.cfg.QueueTau)
	if alpha > 1 {
		alpha = 1
	}
	s.ddEMA += alpha * (dd - s.ddEMA)

	// Calibrate the setpoint once the queue has SETTLED (not at a fixed time): it
	// fills over the first seconds; calibrating mid-ramp captures a wrong level. A
	// hard cap forces it so a genuinely-drifting queue still gets a reference.
	const settleTol = 480 // samples (~10 ms): fast≈slow ⇒ not ramping
	srAlpha := float64(dtNs) / float64(6*s.cfg.QueueTau)
	if srAlpha > 1 {
		srAlpha = 1
	}
	s.ddRef += srAlpha * (dd - s.ddRef)
	if !s.calibrated {
		elapsed := masterNanos - s.startMast
		settled := elapsed >= s.cfg.WarmUp && absf(s.ddEMA-s.ddRef) < settleTol
		forced := elapsed >= 5*s.cfg.WarmUp // hard cap (~15 s at the default)
		if settled || forced {
			s.calibrated = true
			s.setpoint = s.ddEMA
		}
		return s.outPPM // hold at 0 until calibrated
	}

	// PI control. Queue above setpoint ⇒ DAC draining slower than we fill ⇒ produce
	// slower ⇒ negative ppm. The actuator (sink output length) genuinely moves the
	// queue (PLAN-playout-rate-lock), so the integral converges and parks the error
	// at zero — no proportional droop, no ramp to the rail. Kq stays gentle so ±10 ms
	// queue jitter maps to tens of ppm, not the ±300 clamp ("stumbling").
	s.queueErr = s.ddEMA - s.setpoint
	dtSec := float64(dtNs) / 1e9
	s.integral += s.queueErr * dtSec
	// Anti-windup: cap the integral's authority to the output clamp so it can never
	// charge past what the clamp allows (and a transient unwinds promptly).
	if s.cfg.Ki > 0 {
		maxI := s.cfg.ClampPPM / s.cfg.Ki
		if s.integral > maxI {
			s.integral = maxI
		} else if s.integral < -maxI {
			s.integral = -maxI
		}
	}
	target := -(s.cfg.Kq*s.queueErr + s.cfg.Ki*s.integral)
	if target > s.cfg.ClampPPM {
		target = s.cfg.ClampPPM
	} else if target < -s.cfg.ClampPPM {
		target = -s.cfg.ClampPPM
	}

	// Slew toward target at SlewPPM per second.
	maxStep := s.cfg.SlewPPM * float64(dtNs) / 1e9
	d := target - s.outPPM
	if d > maxStep {
		d = maxStep
	} else if d < -maxStep {
		d = -maxStep
	}
	s.outPPM += d
	return s.outPPM
}

// ratePPM returns the last correction (for SinkStats.RatePPM).
func (s *rateServo) ratePPM() float64 { return s.outPPM }

// setpointNs is the calibrated device-queue setpoint in ns (0 until calibrated) —
// telemetry so the per-node queue level and inter-node skew are visible (D64).
func (s *rateServo) setpointNs() int64 {
	return int64(s.setpoint * 1e9 / float64(stream.SampleRate))
}

// reset clears state for a new session.
func (s *rateServo) reset() {
	s.have = false
	s.startMast = 0
	s.lastMast = 0
	s.ddEMA = 0
	s.ddRef = 0
	s.calibrated = false
	s.setpoint = 0
	s.integral = 0
	s.outPPM = 0
	s.queueErr = 0
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
