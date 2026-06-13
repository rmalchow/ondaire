package sink

import (
	"encoding/binary"
	"math"
	"testing"

	"ensemble/internal/stream"
)

// makeFrame builds a canonical PCM frame from a per-sample generator (L,R).
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

func sampleLR(frame []byte, i int) (int16, int16) {
	off := i * stream.Channels * stream.BytesPerSmpl
	l := int16(binary.LittleEndian.Uint16(frame[off : off+2]))
	r := int16(binary.LittleEndian.Uint16(frame[off+2 : off+4]))
	return l, r
}

// TestResamplerOutputLength: process returns exactly the requested outLen samples
// (clamped to ±maxOutDelta), regardless of the fixed FrameSamples input.
func TestResamplerOutputLength(t *testing.T) {
	r := newResampler()
	in := makeFrame(func(i int) (int16, int16) { return int16(i), int16(i) })
	for _, outLen := range []int{stream.FrameSamples, stream.FrameSamples + 1, stream.FrameSamples - 1, stream.FrameSamples + 5} {
		out := r.process(in, outLen)
		if len(out) != outLen*stream.Channels*stream.BytesPerSmpl {
			t.Fatalf("outLen %d: got %d bytes, want %d", outLen, len(out), outLen*stream.Channels*stream.BytesPerSmpl)
		}
	}
	// Out-of-range requests clamp, never panic / over-read the buffer.
	if out := r.process(in, stream.FrameSamples+10_000); len(out) != (stream.FrameSamples+maxOutDelta)*stream.Channels*stream.BytesPerSmpl {
		t.Fatalf("huge outLen not clamped: %d bytes", len(out))
	}
}

// TestResamplerCountsGrounded: the realized rate-match counts reflect REAL output
// surplus/deficit — zero at outLen==FrameSamples, only injects when outLen>frame,
// only drops when outLen<frame, summing |outLen−FrameSamples|, cumulative across reset.
func TestResamplerCountsGrounded(t *testing.T) {
	feed := func(r *resampler, n, outLen int) {
		for f := 0; f < n; f++ {
			base := f * stream.FrameSamples
			r.process(makeFrame(func(i int) (int16, int16) {
				v := int16((base + i) % 1000)
				return v, v
			}), outLen)
		}
	}
	// outLen == FrameSamples: nothing added/removed.
	r := newResampler()
	feed(r, 200, stream.FrameSamples)
	if inj, drop := r.sampleStats(); inj != 0 || drop != 0 {
		t.Fatalf("outLen==frame: injected=%d dropped=%d, want 0/0", inj, drop)
	}
	// outLen > frame (upsample, feed a faster DAC): only INJECTS, 1 per frame.
	r = newResampler()
	feed(r, 300, stream.FrameSamples+1)
	inj, drop := r.sampleStats()
	if inj != 300 || drop != 0 { // first call primes (still counts: net applies post-prime)
		// prime returns before the inject accounting, so 299 of the 300 count.
		if !(inj == 299 && drop == 0) {
			t.Fatalf("outLen>frame: injected=%d dropped=%d, want ≈299 inj, 0 drop", inj, drop)
		}
	}
	// outLen < frame: only DROPS.
	r = newResampler()
	feed(r, 300, stream.FrameSamples-1)
	inj, drop = r.sampleStats()
	if drop == 0 || inj != 0 {
		t.Fatalf("outLen<frame: injected=%d dropped=%d, want drop>0 inj=0", inj, drop)
	}
	// Counters survive reset (lifetime total for the API).
	before := drop
	r.reset()
	if _, d2 := r.sampleStats(); d2 != before {
		t.Fatalf("reset zeroed the lifetime counter: %d -> %d", before, d2)
	}
}

// TestResamplerUnitDelayedIdentity: at outLen==FrameSamples the resampler is a pure
// 1-frame delay (frame 0 silence, output frame f == input frame f-1).
func TestResamplerUnitDelayedIdentity(t *testing.T) {
	r := newResampler()
	var frames [][]byte
	for f := 0; f < 5; f++ {
		base := f * stream.FrameSamples
		frames = append(frames, makeFrame(func(i int) (int16, int16) {
			v := int16((base + i) % 1000)
			return v, -v
		}))
	}
	var outs [][]byte
	for _, in := range frames {
		o := make([]byte, stream.FrameBytes)
		copy(o, r.process(in, stream.FrameSamples))
		outs = append(outs, o)
	}
	for i := 0; i < stream.FrameBytes; i++ {
		if outs[0][i] != 0 {
			t.Fatalf("frame 0 must be silence; byte %d = %d", i, outs[0][i])
		}
	}
	for f := 1; f < 5; f++ {
		for i := 0; i < stream.FrameSamples; i++ {
			il, ir := sampleLR(frames[f-1], i)
			ol, or := sampleLR(outs[f], i)
			if il != ol || ir != or {
				t.Fatalf("frame %d sample %d: want prev-frame (%d,%d) got (%d,%d)", f, i, il, ir, ol, or)
			}
		}
	}
}

// TestResamplerNoSeamGlitch is the regression test for the 50 Hz seam buzz: with a
// sustained rate correction (outLen ≠ FrameSamples) the reconstructed output of a
// continuous 1 kHz sine must have NO curvature spike at the frame boundaries.
func TestResamplerNoSeamGlitch(t *testing.T) {
	r := newResampler()
	const outLen = stream.FrameSamples + 1 // sustained upsample (servo-class)
	freq, amp := 1000.0, 9000.0
	const frames = 12
	var out []int16
	for f := 0; f < frames; f++ {
		base := f * stream.FrameSamples
		in := makeFrame(func(i int) (int16, int16) {
			v := int16(amp * math.Sin(2*math.Pi*freq*float64(base+i)/float64(stream.SampleRate)))
			return v, v
		})
		o := r.process(in, outLen)
		for i := 0; i < outLen; i++ {
			l, _ := sampleLR(o, i)
			out = append(out, l)
		}
	}
	start := 2 * outLen
	var maxCurv float64
	for k := start + 1; k < len(out)-1; k++ {
		c := math.Abs(float64(out[k+1]) - 2*float64(out[k]) + float64(out[k-1]))
		if c > maxCurv {
			maxCurv = c
		}
	}
	if maxCurv > 400 {
		t.Fatalf("curvature spike %.0f (clean sine ~154) — seam glitch present", maxCurv)
	}
}

func TestResamplerSilenceStaysSilence(t *testing.T) {
	r := newResampler()
	silence := make([]byte, stream.FrameBytes)
	for f := 0; f < 3; f++ {
		out := r.process(silence, stream.FrameSamples+1)
		for i := 0; i < len(out); i++ {
			if out[i] != 0 {
				t.Fatalf("frame %d byte %d non-zero: %d", f, i, out[i])
			}
		}
	}
}

func TestResamplerStereoIndependence(t *testing.T) {
	r := newResampler()
	in := makeFrame(func(i int) (int16, int16) {
		return int16(i % 500), int16(-(i % 500))
	})
	r.process(in, stream.FrameSamples)        // frame 0 → silence
	out := r.process(in, stream.FrameSamples) // frame 1 → frame 0 (this same input) delayed
	for i := 0; i < stream.FrameSamples; i++ {
		ol, or := sampleLR(out, i)
		if ol != int16(i%500) || or != int16(-(i%500)) {
			t.Fatalf("sample %d L/R cross-contaminated: (%d,%d)", i, ol, or)
		}
	}
}

// TestResamplerCarryBounded: because input consumption is fixed at one frame, the
// carry length is invariant at leadPad+FrameSamples regardless of outLen — no drift,
// no growth, ever.
func TestResamplerCarryBounded(t *testing.T) {
	r := newResampler()
	in := makeFrame(func(i int) (int16, int16) { return int16(i % 777), int16(i % 777) })
	r.process(in, stream.FrameSamples) // prime
	for f := 0; f < 2000; f++ {
		outLen := stream.FrameSamples + (f%3 - 1) // cycle 959/960/961
		r.process(in, outLen)
		for ch := 0; ch < stream.Channels; ch++ {
			if len(r.carry[ch]) != leadPad+stream.FrameSamples {
				t.Fatalf("frame %d ch %d: carry = %d, want %d (invariant)", f, ch, len(r.carry[ch]), leadPad+stream.FrameSamples)
			}
		}
	}
}
