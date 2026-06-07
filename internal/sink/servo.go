package sink

import "ensemble/internal/stream"

// servoConfig holds the rate-servo parameters. The servo holds the output
// DEVICE QUEUE at a setpoint with a proportional controller: that is the only
// software-observable signal for DAC crystal drift (the playout scheduler is
// master-clock-locked, so consumed-vs-master measures the scheduler, not the
// DAC — the queue depth is where the DAC's true rate shows up).
type servoConfig struct {
	WarmUp   int64   // ns to wait before calibrating (queue fill + EMA settle)
	QueueTau int64   // device-queue smoothing time constant, ns
	Kq       float64 // gain: ppm of rate correction per sample of queue error
	ClampPPM float64 // output clamp, ± (a real crystal is well under this)
	SlewPPM  float64 // max |Δoutput| per second, ppm/s
}

func defaultServoConfig() servoConfig {
	return servoConfig{
		WarmUp:   3_000_000_000, // 3 s: let the device queue fill and the EMA settle
		QueueTau: 2_000_000_000, // 2 s: smooths the Pi's ±10ms snd_pcm_delay jitter
		Kq:       1.5, // loop time-constant ~13s (rate error integrates into
		//                queue depth only slowly); the SlewPPM limit below is
		//                the real noise filter, so a brisk Kq is safe.
		ClampPPM: 300,           // ±0.03% — covers any real crystal; inaudible
		SlewPPM:  30,            // ppm/s — gentle, no audible rate steps
	}
}

// rateServo turns a stream of (consumed, master, deviceDelay) observations into
// a playback-rate correction in ppm. One per session; reset on generation
// change. Pure: no goroutine, no clock, no locking (the Playout mutex guards
// it). Without a DelayReporter backend (deviceDelay ok=false) it returns 0 —
// DAC drift is then unobservable and accepted (the player owns its own clock).
type rateServo struct {
	cfg        servoConfig
	have       bool    // first observation seen (EMA + clock seeded)
	startMast  int64   // master ns of the first observation (warmup origin)
	lastMast   int64   // master ns of the previous observation (per-step dt)
	ddEMA      float64 // smoothed device-queue depth, samples
	calibrated bool    // setpoint captured (warmup elapsed)
	setpoint   float64 // target device-queue depth, samples
	outPPM     float64 // current correction, ppm (slew-limited, clamped)

	// telemetry (read by the sink's 1 Hz debug line)
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
		s.startMast = masterNanos
		s.lastMast = masterNanos
		return s.outPPM // 0
	}

	dtNs := masterNanos - s.lastMast
	s.lastMast = masterNanos
	if dtNs <= 0 {
		return s.outPPM
	}

	// Smooth the device-queue depth (EMA, time-constant QueueTau). The Pi's
	// snd_pcm_delay swings ±10ms (~±480 samples) per read; only its slow trend
	// carries the drift signal.
	alpha := float64(dtNs) / float64(s.cfg.QueueTau)
	if alpha > 1 {
		alpha = 1
	}
	s.ddEMA += alpha * (dd - s.ddEMA)

	// Warmup: the queue fills 0→full at session start; calibrate the setpoint
	// to wherever it settles, so a steady queue reads zero error and only DRIFT
	// moves it off setpoint.
	if masterNanos-s.startMast < s.cfg.WarmUp {
		return s.outPPM // hold at 0 while filling
	}
	if !s.calibrated {
		s.calibrated = true
		s.setpoint = s.ddEMA
		return s.outPPM
	}

	// Proportional control. Queue above setpoint ⇒ the DAC is draining slower
	// than we fill ⇒ DAC slow ⇒ produce slower ⇒ negative ppm. In steady state
	// the queue stops drifting and outPPM equals the DAC's drift correction.
	s.queueErr = s.ddEMA - s.setpoint
	target := -s.cfg.Kq * s.queueErr
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

// reset clears state for a new session.
func (s *rateServo) reset() {
	s.have = false
	s.startMast = 0
	s.lastMast = 0
	s.ddEMA = 0
	s.calibrated = false
	s.setpoint = 0
	s.outPPM = 0
	s.queueErr = 0
}
