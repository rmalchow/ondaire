package sink

import (
	"encoding/binary"
	"math"
	"testing"

	"ondaire/internal/stream"
)

// These tests cover the PULL resampler (PLAN-dac-pull-phase-lock): the caller
// feeds whole input frames (feed) and pulls a fixed FrameSamples of output per
// process(ratio) call. The rate correction lives entirely in `ratio` (input
// samples advanced per output sample); the output frame size never changes.
// Everything here is pure and deterministic — no clock, no goroutine.

// makeFrame builds a canonical PCM frame from a per-sample (L,R) generator.
func makeFrame(gen func(i int) (int16, int16)) []byte {
	f := make([]byte, stream.FrameBytes)
	for i := 0; i < stream.FrameSamples; i++ {
		l, r := gen(i)
		off := i * stream.Channels * stream.BytesPerSmpl
		binary.LittleEndian.PutUint16(f[off:off+2], uint16(l))
		binary.LittleEndian.PutUint16(f[off+2:off+4], uint16(r))
	}
	return f
}

// sampleLR decodes the i-th interleaved L/R sample pair from a frame.
func sampleLR(frame []byte, i int) (int16, int16) {
	off := i * stream.Channels * stream.BytesPerSmpl
	l := int16(binary.LittleEndian.Uint16(frame[off : off+2]))
	r := int16(binary.LittleEndian.Uint16(frame[off+2 : off+4]))
	return l, r
}

// feedAvail tops the resampler up to at least needInput samples ahead of its
// cursor (the engine's feedLocked invariant), pulling fresh frames from gen.
func feedAvail(r *resampler, next func() []byte) {
	for r.inputAvail() < needInput {
		r.feed(next())
	}
}

// TestResamplerOutputLength: process always emits exactly one FrameBytes frame,
// independent of the ratio (the rate correction is in the cursor advance, not
// the output size — the ALSA backend requires fixed-size writes).
func TestResamplerOutputLength(t *testing.T) {
	r := newResampler()
	frame := makeFrame(func(i int) (int16, int16) { return int16(i), int16(-i) })
	for _, ratio := range []float64{0.9990, 1.0, 1.0010, 0.5, 2.0} {
		feedAvail(r, func() []byte { return frame })
		out := r.process(ratio)
		if len(out) != stream.FrameBytes {
			t.Fatalf("ratio %.4f: got %d bytes, want %d", ratio, len(out), stream.FrameBytes)
		}
	}
}

// TestResamplerCursorAdvance: the cursor (and cumulative consumed) advances by
// exactly FrameSamples*ratio per process(), so consumedSamples ≈ Σ FrameSamples*ratio
// to well under one sample of accumulated float error. consumedSamples is the
// engine's fedPTS reference — drift here is drift in the phase observation.
func TestResamplerCursorAdvance(t *testing.T) {
	r := newResampler()
	frame := makeFrame(func(i int) (int16, int16) { return int16(i % 1000), int16(i % 1000) })
	const ratio = 1.0003
	const n = 5000
	var want float64
	for k := 0; k < n; k++ {
		feedAvail(r, func() []byte { return frame })
		r.process(ratio)
		want += float64(stream.FrameSamples) * ratio
	}
	if d := math.Abs(r.consumedSamples() - want); d >= 1 {
		t.Fatalf("consumed=%.4f want=%.4f (|Δ|=%.4f, must be <1 sample)", r.consumedSamples(), want, d)
	}
}

// TestResamplerCarryBounded: over 1e5 process() calls at servo-class ratios the
// input buffer never grows without bound. Each process drops the whole consumed
// samples from the front (keeping leadPad lookback), so the live buffer stays
// near one frame plus the feed top-up — bounded by ~2*FrameSamples regardless of
// ratio sign. This is the regression guard against a slow buffer leak.
func TestResamplerCarryBounded(t *testing.T) {
	r := newResampler()
	frame := makeFrame(func(i int) (int16, int16) { return int16(i % 777), int16(-(i % 777)) })
	ratios := []float64{0.9997, 1.0, 1.0003}
	// Upper bound: the feed loop tops up to >= needInput (≈ one frame + taps),
	// each feed adds one whole frame, so the buffer can reach roughly two frames
	// plus the retained leadPad/lookahead. Assert it never exceeds that envelope.
	bound := 2*stream.FrameSamples + leadPad + lookahead + needInput
	for k := 0; k < 100_000; k++ {
		feedAvail(r, func() []byte { return frame })
		r.process(ratios[k%len(ratios)])
		for ch := 0; ch < stream.Channels; ch++ {
			if l := len(r.in[ch]); l > bound {
				t.Fatalf("call %d ch %d: input buffer = %d, exceeds bound %d (leak)", k, ch, l, bound)
			}
		}
	}
}

// TestResamplerUnitIdentity: at ratio==1 the resampler is ~identity. The Catmull-Rom
// kernel needs p2,p3 ahead of the cursor (lookahead=2), so the FIRST full-fidelity
// frame is only available after TWO frames are fed; from then on process(1.0) at an
// integer cursor reproduces the fed samples within a tiny rounding epsilon (the
// kernel passes exactly through its control points at t==0, so this is exact here).
func TestResamplerUnitIdentity(t *testing.T) {
	r := newResampler()
	f0 := makeFrame(func(i int) (int16, int16) { return int16(i % 200), int16(-(i % 200)) })
	f1 := makeFrame(func(i int) (int16, int16) { return int16((i + 7) % 200), int16(-((i + 7) % 200)) })
	r.feed(f0)
	r.feed(f1) // lookahead: must have p2/p3 of frame 0 available
	out := r.process(1.0)
	for i := 0; i < stream.FrameSamples; i++ {
		il, ir := sampleLR(f0, i)
		ol, or := sampleLR(out, i)
		if ol != il || or != ir {
			t.Fatalf("sample %d: want (%d,%d) got (%d,%d) — not identity at ratio 1", i, il, ir, ol, or)
		}
	}
}

// TestResamplerNoSeamGlitch is the regression guard for the 50 Hz seam buzz: under
// a SUSTAINED rate correction (ratio≠1) the reconstructed output of a continuous
// 1 kHz sine must have NO curvature spike at the frame boundaries. We measure the
// discrete second difference |y[k+1]-2y[k]+y[k-1]| (curvature) across the stream;
// a clean reconstructed sine sits near a small baseline and a seam discontinuity
// would show up as an isolated spike many times that baseline.
func TestResamplerNoSeamGlitch(t *testing.T) {
	r := newResampler()
	const ratio = 1.0003 // sustained servo-class stretch
	const freq, amp = 1000.0, 9000.0
	const frames = 16
	// Continuous sine across all frames: sample index advances globally so the
	// phase is continuous frame-to-frame (a real audio stream).
	gi := 0
	next := func() []byte {
		f := makeFrame(func(i int) (int16, int16) {
			v := int16(amp * math.Sin(2*math.Pi*freq*float64(gi+i)/float64(stream.SampleRate)))
			return v, v
		})
		gi += stream.FrameSamples
		return f
	}
	var out []int16
	for f := 0; f < frames; f++ {
		feedAvail(r, next)
		o := r.process(ratio)
		for i := 0; i < stream.FrameSamples; i++ {
			l, _ := sampleLR(o, i)
			out = append(out, l)
		}
	}
	// Skip the first two output frames (priming transient at the cursor origin).
	start := 2 * stream.FrameSamples
	var maxCurv float64
	for k := start + 1; k < len(out)-1; k++ {
		c := math.Abs(float64(out[k+1]) - 2*float64(out[k]) + float64(out[k-1]))
		if c > maxCurv {
			maxCurv = c
		}
	}
	// A clean 1 kHz sine at 48 kHz has curvature ≈ amp*(2π·f/fs)² ≈ 9000*0.0171 ≈ 154.
	// A seam discontinuity would spike to many hundreds; allow generous headroom.
	if maxCurv > 400 {
		t.Fatalf("curvature spike %.0f (clean sine ~154) — seam glitch present", maxCurv)
	}
}

// TestResamplerSilenceStaysSilence: silence in → silence out, at any ratio (no
// kernel ringing or DC offset injected by the interpolation).
func TestResamplerSilenceStaysSilence(t *testing.T) {
	r := newResampler()
	silence := make([]byte, stream.FrameBytes)
	for f := 0; f < 8; f++ {
		feedAvail(r, func() []byte { return silence })
		out := r.process(1.0007)
		for i := range out {
			if out[i] != 0 {
				t.Fatalf("frame %d byte %d non-zero: %d", f, i, out[i])
			}
		}
	}
}

// TestResamplerStereoIndependence: the two channels are resampled independently;
// an L ramp and an inverted R ramp must not cross-contaminate (the per-channel
// buffers are separate, the interleave is correct on the way out).
func TestResamplerStereoIndependence(t *testing.T) {
	r := newResampler()
	frame := makeFrame(func(i int) (int16, int16) { return int16(i % 500), int16(-(i % 500)) })
	r.feed(frame)
	r.feed(frame)
	out := r.process(1.0)
	for i := 0; i < stream.FrameSamples; i++ {
		ol, or := sampleLR(out, i)
		wantL := int16(i % 500)
		wantR := int16(-(i % 500))
		if ol != wantL || or != wantR {
			t.Fatalf("sample %d L/R cross-contaminated: got (%d,%d) want (%d,%d)", i, ol, or, wantL, wantR)
		}
	}
}

// TestResamplerSampleStatsGrounded: the realized inject/drop accounting reflects
// the cursor's surplus/deficit vs one output frame. ratio>1 consumes MORE input
// per frame (compress, catching up) → drops; ratio<1 consumes fewer → injects;
// ratio==1 does neither. The totals are cumulative and survive reset (lifetime
// counters for the API).
func TestResamplerSampleStatsGrounded(t *testing.T) {
	frame := makeFrame(func(i int) (int16, int16) { return int16(i % 1000), int16(i % 1000) })
	// ratio == 1: nothing injected or dropped.
	r := newResampler()
	for k := 0; k < 300; k++ {
		feedAvail(r, func() []byte { return frame })
		r.process(1.0)
	}
	if inj, drop := r.sampleStats(); inj != 0 || drop != 0 {
		t.Fatalf("ratio==1: inj=%d drop=%d, want 0/0", inj, drop)
	}
	// ratio > 1 (compress): only DROPS accumulate.
	r = newResampler()
	for k := 0; k < 300; k++ {
		feedAvail(r, func() []byte { return frame })
		r.process(1.0005)
	}
	if inj, drop := r.sampleStats(); inj != 0 || drop == 0 {
		t.Fatalf("ratio>1: inj=%d drop=%d, want inj=0 drop>0", inj, drop)
	}
	// ratio < 1 (stretch): only INJECTS accumulate; counter survives reset.
	r = newResampler()
	for k := 0; k < 300; k++ {
		feedAvail(r, func() []byte { return frame })
		r.process(0.9995)
	}
	inj, drop := r.sampleStats()
	if inj == 0 || drop != 0 {
		t.Fatalf("ratio<1: inj=%d drop=%d, want inj>0 drop=0", inj, drop)
	}
	before := inj
	r.reset()
	if i2, _ := r.sampleStats(); i2 != before {
		t.Fatalf("reset zeroed lifetime inject counter: %d -> %d", before, i2)
	}
}

// TestResamplerResetClearsHistory: reset drops the live buffer and unprimes, so the
// next session re-seeds leadPad lookback from its own first frame (no bleed from
// the previous gen), while the lifetime inject/drop totals survive.
func TestResamplerResetClearsHistory(t *testing.T) {
	r := newResampler()
	loud := makeFrame(func(i int) (int16, int16) { return 12000, -12000 })
	r.feed(loud)
	r.feed(loud)
	r.process(1.0)
	r.reset()
	if r.primed {
		t.Fatal("reset left the resampler primed")
	}
	if r.inputAvail() != 0 {
		t.Fatalf("reset left %d input samples available", r.inputAvail())
	}
	// New session of silence reproduces silence — no leftover loud history.
	silence := make([]byte, stream.FrameBytes)
	r.feed(silence)
	r.feed(silence)
	out := r.process(1.0)
	for i := range out {
		if out[i] != 0 {
			t.Fatalf("post-reset byte %d = %d, want silence (history bled through)", i, out[i])
		}
	}
}
