package sink

import (
	"math"
	"testing"

	"ensemble/internal/stream"
)

// fastServoCfg converges quickly for tests (short warmup/tau, brisk slew).
func fastServoCfg() servoConfig {
	return servoConfig{
		WarmUp:   200_000_000, // 200 ms
		QueueTau: 100_000_000, // 100 ms
		Kq:       2.0,
		ClampPPM: 300,
		SlewPPM:  4000, // ppm/s — fast so tests converge in a few hundred frames
	}
}

// driveServoQueue is a CLOSED-LOOP device-queue simulation. The backend's DAC
// drains the device queue at dacPPM off nominal; the servo's correction sets
// the rate at which the resampler PRODUCES into that queue. deviceDelay is the
// live queue depth. noise adds ±noise samples of measurement jitter (the Pi's
// snd_pcm_delay). In steady state the servo settles where production == drain,
// i.e. outPPM ≈ dacPPM, holding the queue near its calibrated setpoint.
func driveServoQueue(s *rateServo, dacPPM float64, steps int, noise float64) float64 {
	var master int64
	var consumed float64
	queue := float64(stream.SampleRate) / 5 // start ~200 ms filled
	var out float64
	seed := int64(12345)
	rnd := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(uint64(seed)>>40) / float64(uint64(1)<<24) // ~[0,1)
	}
	for i := 0; i < steps; i++ {
		master += stream.FrameNanos
		produced := float64(stream.FrameSamples) * (1 + out/1e6)
		drained := float64(stream.FrameSamples) * (1 + dacPPM/1e6)
		queue += produced - drained
		consumed += produced
		measured := queue
		if noise > 0 {
			measured += (2*rnd() - 1) * noise
		}
		ddNs := int64(measured / float64(stream.SampleRate) * 1e9)
		out = s.observe(int64(consumed), master, ddNs, true)
	}
	return out
}

func TestServoConvergesOnFastDAC(t *testing.T) {
	s := newRateServo(fastServoCfg())
	out := driveServoQueue(s, 200, 8000, 0) // DAC 200ppm fast → produce 200ppm faster
	if math.Abs(out-200) > 30 {
		t.Fatalf("expected correction ≈ +200 ppm, got %.1f", out)
	}
}

func TestServoConvergesNegative(t *testing.T) {
	s := newRateServo(fastServoCfg())
	out := driveServoQueue(s, -150, 8000, 0)
	if math.Abs(out-(-150)) > 30 {
		t.Fatalf("expected correction ≈ -150 ppm, got %.1f", out)
	}
}

func TestServoConvergesUnderNoise(t *testing.T) {
	// The Pi's snd_pcm_delay swings ±10ms (~480 samples) per read; the servo
	// must still converge to the true drift, not chase the noise. Noise
	// rejection is the job of the queue EMA (2s) AND the slew limit, so use
	// the production-like slew here, not fastServoCfg's quick-converge slew.
	cfg := servoConfig{
		WarmUp:   500_000_000,
		QueueTau: 2_000_000_000,
		Kq:       1.5,
		ClampPPM: 300,
		SlewPPM:  60, // production-class; slew IS the final noise filter
	}
	s := newRateServo(cfg)
	out := driveServoQueue(s, 120, 60000, 480) // ~20 min sim (pure math, instant)
	if math.Abs(out-120) > 40 {
		t.Fatalf("under ±480-sample noise expected ≈ +120 ppm, got %.1f", out)
	}
}

func TestServoClampsPPM(t *testing.T) {
	s := newRateServo(fastServoCfg())
	out := driveServoQueue(s, 5000, 8000, 0) // absurd drift → clamp
	if out < -300.0001 || out > 300.0001 {
		t.Fatalf("output %.1f outside ±300 clamp", out)
	}
	if out < 250 {
		t.Fatalf("expected correction near +300 clamp, got %.1f", out)
	}
}

func TestServoNoSignalWithoutDelayReporter(t *testing.T) {
	// ok=false (exec/pw-play: no snd_pcm_delay) ⇒ no drift signal ⇒ stay at 0.
	s := newRateServo(fastServoCfg())
	var master, consumed int64
	for i := 0; i < 500; i++ {
		master += stream.FrameNanos
		consumed += stream.FrameSamples
		if out := s.observe(consumed, master, 0, false); out != 0 {
			t.Fatalf("ok=false must return 0, got %.3f", out)
		}
	}
}

func TestServoSlewLimited(t *testing.T) {
	cfg := fastServoCfg()
	cfg.SlewPPM = 50 // ppm/s; per 20ms frame ⇒ ≤1 ppm/step
	s := newRateServo(cfg)
	var master int64
	var consumed float64
	queue := float64(stream.SampleRate) / 5
	prev := 0.0
	maxStep := cfg.SlewPPM * float64(stream.FrameNanos) / 1e9
	for i := 0; i < 1500; i++ {
		master += stream.FrameNanos
		produced := float64(stream.FrameSamples) * (1 + prev/1e6)
		queue += produced - float64(stream.FrameSamples)*(1+9000/1e6) // huge drift
		consumed += produced
		out := s.observe(int64(consumed), master, int64(queue/float64(stream.SampleRate)*1e9), true)
		if math.Abs(out-prev) > maxStep+1e-6 {
			t.Fatalf("step %d: |Δout|=%.4f exceeds slew %.4f", i, math.Abs(out-prev), maxStep)
		}
		prev = out
	}
}

func TestServoResetClears(t *testing.T) {
	s := newRateServo(fastServoCfg())
	driveServoQueue(s, 200, 2000, 0)
	s.reset()
	if s.outPPM != 0 || s.calibrated || s.have {
		t.Fatalf("reset incomplete: %+v", *s)
	}
}
