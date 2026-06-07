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

func TestResamplerIdentityAtUnitRate(t *testing.T) {
	r := newResampler()
	r.setRate(0) // ppm=0 → rate 1
	for f := 0; f < 4; f++ {
		base := f * stream.FrameSamples
		in := makeFrame(func(i int) (int16, int16) {
			v := int16((base + i) % 1000)
			return v, -v
		})
		out := r.process(in)
		for i := 0; i < stream.FrameSamples; i++ {
			il, ir := sampleLR(in, i)
			ol, or := sampleLR(out, i)
			if il != ol || ir != or {
				t.Fatalf("frame %d sample %d: in (%d,%d) out (%d,%d)", f, i, il, ir, ol, or)
			}
		}
	}
}

func TestResamplerOutputFrameSize(t *testing.T) {
	r := newResampler()
	r.setRate(300)
	in := makeFrame(func(i int) (int16, int16) { return int16(i), int16(i) })
	out := r.process(in)
	if len(out) != stream.FrameBytes {
		t.Fatalf("output %d bytes, want %d", len(out), stream.FrameBytes)
	}
}

func TestResamplerContinuityAcrossFrames(t *testing.T) {
	r := newResampler()
	r.setRate(300) // small stretch
	// A continuous low-frequency sine spanning multiple frames.
	freq := 220.0
	amp := 10000.0
	sineAt := func(n int) int16 {
		return int16(amp * math.Sin(2*math.Pi*freq*float64(n)/float64(stream.SampleRate)))
	}
	var prevLast int16
	var prevSlope float64
	for f := 0; f < 6; f++ {
		base := f * stream.FrameSamples
		in := makeFrame(func(i int) (int16, int16) {
			v := sineAt(base + i)
			return v, v
		})
		out := r.process(in)
		firstL, _ := sampleLR(out, 0)
		if f > 0 {
			// The seam jump must be comparable to the local slope, not a click.
			seamJump := math.Abs(float64(firstL - prevLast))
			if seamJump > math.Abs(prevSlope)+200 {
				t.Fatalf("frame %d seam discontinuity: jump=%.0f localSlope=%.0f", f, seamJump, prevSlope)
			}
		}
		last, _ := sampleLR(out, stream.FrameSamples-1)
		secondLast, _ := sampleLR(out, stream.FrameSamples-2)
		prevLast = last
		prevSlope = float64(last - secondLast)
	}
}

func TestResamplerSilenceStaysSilence(t *testing.T) {
	r := newResampler()
	r.setRate(200)
	silence := make([]byte, stream.FrameBytes)
	for f := 0; f < 3; f++ {
		out := r.process(silence)
		for i := 0; i < stream.FrameBytes; i++ {
			if out[i] != 0 {
				t.Fatalf("frame %d byte %d non-zero: %d", f, i, out[i])
			}
		}
	}
}

func TestResamplerStereoIndependence(t *testing.T) {
	r := newResampler()
	r.setRate(0) // unity for an exact check
	in := makeFrame(func(i int) (int16, int16) {
		return int16(i % 500), int16(-(i % 500))
	})
	out := r.process(in)
	for i := 0; i < stream.FrameSamples; i++ {
		ol, or := sampleLR(out, i)
		if ol != int16(i%500) || or != int16(-(i%500)) {
			t.Fatalf("sample %d L/R cross-contaminated: (%d,%d)", i, ol, or)
		}
	}
}

func TestResamplerCursorBounded(t *testing.T) {
	// Over many frames at a sustained correction the cursor must stay bounded
	// (the leftover fraction is kept; whole input samples are discarded). A
	// run-away cursor would mean unbounded carry growth / precision loss.
	r := newResampler()
	r.setRate(400)
	in := makeFrame(func(i int) (int16, int16) { return int16(i % 777), int16(i % 777) })
	for f := 0; f < 2000; f++ {
		r.process(in)
		if r.cursor < 0 || r.cursor >= float64(leadPad)+2 {
			t.Fatalf("frame %d: cursor %.4f left bounded range", f, r.cursor)
		}
		for ch := 0; ch < stream.Channels; ch++ {
			if len(r.carry[ch]) > stream.FrameSamples+leadPad+8 {
				t.Fatalf("frame %d ch %d: carry grew to %d", f, ch, len(r.carry[ch]))
			}
		}
	}
}
