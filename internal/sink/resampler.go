package sink

import (
	"encoding/binary"

	"ensemble/internal/stream"
)

// resampler bridges the master-clock input stream to the local DAC's rate. Each
// call consumes EXACTLY one input frame (FrameSamples of master-paced content) and
// produces a caller-chosen number of output samples `outLen`, interpolated with a
// 4-tap Catmull-Rom kernel run independently per channel (interleaved L/R).
//
// This is the rate actuator (PLAN-playout-rate-lock): the servo holds the output
// device queue at its setpoint by choosing outLen. outLen > FrameSamples upsamples
// (stretches one frame of content over more DAC samples → feeds a faster DAC);
// outLen < FrameSamples downsamples. Because input consumption is fixed at one
// frame, the carry never drifts and there is no whole-frame discharge — the rate
// correction is the output COUNT, realized at single-sample granularity.
//
// Catmull-Rom for fractional position t∈[0,1) between samples p1 and p2, with
// neighbours p0 (before) and p3 (after):
//
//	y(t) = 0.5 * ( (2*p1)
//	             + (-p0 + p2)*t
//	             + (2*p0 - 5*p1 + 4*p2 - p3)*t^2
//	             + (-p0 + 3*p1 - 3*p2 + p3)*t^3 )
//
// Output sample k reads input position leadPad + k*step, step = FrameSamples/outLen.
//
// Bookkeeping: the carry holds [leadPad history][previous frame]; each call appends
// the new frame (so the previous frame's tail has real p2/p3 lookahead), emits
// outLen interpolated samples of the PREVIOUS frame, then drops that whole frame —
// one frame (20 ms) of latency, well within bufferMs. The seam always has a real
// 4-tap window, so no per-frame clamp (the old 50 Hz buzz).
type resampler struct {
	// carry holds [leadPad history][one held frame] of input samples per channel.
	// We keep leadPad leading samples so p0 (pos-1) is valid at the seam.
	carry  [stream.Channels][]int32
	primed bool

	out  []byte
	work [stream.Channels][]int32 // reusable per-channel input scratch

	// Realized rate-match accounting (per-channel sample units, = time): the net
	// samples the rate correction added to (injected) or removed from (dropped) the
	// output stream, = Σ(outLen − FrameSamples). NOT the commanded ppm: this is what
	// reached the DAC. Cumulative for the resampler's life (survives reset) so the
	// API exposes a running total; once the servo locks it tracks the DAC offset.
	injected uint64 // samples added to the output (outLen > FrameSamples)
	dropped  uint64 // samples removed from the output (outLen < FrameSamples)
}

// leadPad is how many leading carry samples we always keep so p0 (pos-1) and the
// Catmull-Rom window are valid at the seam.
const leadPad = 3

// maxOutDelta bounds |outLen − FrameSamples| per frame: ±16 samples ≈ ±1.6%, far
// beyond any real crystal, but caps a transient/garbage command to a gentle ramp.
const maxOutDelta = 16

func newResampler() *resampler {
	r := &resampler{out: make([]byte, (stream.FrameSamples+maxOutDelta)*stream.Channels*stream.BytesPerSmpl)}
	for ch := 0; ch < stream.Channels; ch++ {
		r.carry[ch] = make([]int32, 0, 2*stream.FrameSamples+leadPad+4)
		r.work[ch] = make([]int32, stream.FrameSamples)
	}
	return r
}

// process consumes input frame `in` (exactly FrameBytes) and returns `outLen`
// output samples per channel (outLen clamped to FrameSamples ± maxOutDelta). The
// number of input samples consumed is always exactly FrameSamples — outLen sets
// the resample ratio (FrameSamples/outLen), not the input consumption.
func (r *resampler) process(in []byte, outLen int) []byte {
	const n = stream.FrameSamples
	if outLen < n-maxOutDelta {
		outLen = n - maxOutDelta
	} else if outLen > n+maxOutDelta {
		outLen = n + maxOutDelta
	}
	outBytes := outLen * stream.Channels * stream.BytesPerSmpl

	// Decode the input frame into per-channel scratch.
	for i := 0; i < n; i++ {
		base := i * stream.Channels * stream.BytesPerSmpl
		for ch := 0; ch < stream.Channels; ch++ {
			off := base + ch*stream.BytesPerSmpl
			r.work[ch][i] = int32(int16(binary.LittleEndian.Uint16(in[off : off+2])))
		}
	}

	if !r.primed {
		// First frame: seed [leadPad history][this frame] as the held lookahead and
		// emit silence (one-time 20 ms startup latency, masked by the playout buffer).
		// From the next call on, each output frame is the PREVIOUS input frame, fully
		// resampled with a real 4-tap window at every seam.
		for ch := 0; ch < stream.Channels; ch++ {
			r.carry[ch] = r.carry[ch][:0]
			s0 := r.work[ch][0]
			for j := 0; j < leadPad; j++ {
				r.carry[ch] = append(r.carry[ch], s0)
			}
			r.carry[ch] = append(r.carry[ch], r.work[ch]...)
		}
		r.primed = true
		for i := 0; i < outBytes; i++ {
			r.out[i] = 0
		}
		return r.out[:outBytes]
	}

	// Steady state: carry holds [leadPad][previous frame]. Append the new frame so
	// the previous frame's tail has real p2/p3, emit outLen samples spanning the
	// previous frame's content, then drop that whole frame.
	for ch := 0; ch < stream.Channels; ch++ {
		r.carry[ch] = append(r.carry[ch], r.work[ch]...)
	}

	step := float64(n) / float64(outLen)
	for ch := 0; ch < stream.Channels; ch++ {
		buf := r.carry[ch]
		last := len(buf) - 1
		for k := 0; k < outLen; k++ {
			pos := float64(leadPad) + float64(k)*step
			idx := int(pos)
			t := pos - float64(idx)
			p0 := atIdx(buf, idx-1, last)
			p1 := atIdx(buf, idx, last)
			p2 := atIdx(buf, idx+1, last)
			p3 := atIdx(buf, idx+2, last)
			y := catmullRom(p0, p1, p2, p3, t)
			v := clampInt16(y)
			off := (k*stream.Channels + ch) * stream.BytesPerSmpl
			binary.LittleEndian.PutUint16(r.out[off:off+2], uint16(v))
		}
	}

	// Consume exactly one input frame (the one we just rendered), keeping the
	// leadPad tail as history for the next seam. The held lookahead is the new frame.
	for ch := 0; ch < stream.Channels; ch++ {
		buf := r.carry[ch]
		copy(buf, buf[n:])
		r.carry[ch] = buf[:len(buf)-n]
	}

	if d := outLen - n; d > 0 {
		r.injected += uint64(d)
	} else if d < 0 {
		r.dropped += uint64(-d)
	}
	return r.out[:outBytes]
}

// sampleStats returns the cumulative realized rate-match counts (per-channel
// sample units): samples added to, and removed from, the output stream.
func (r *resampler) sampleStats() (injected, dropped uint64) {
	return r.injected, r.dropped
}

// atIdx returns buf[i], clamping to the buffer ends (the read window stays within
// the held frame + leadPad, so clamping only ever touches the final lookahead tap).
func atIdx(buf []int32, i, last int) int32 {
	if i < 0 {
		i = 0
	} else if i > last {
		i = last
	}
	return buf[i]
}

func catmullRom(p0, p1, p2, p3 int32, t float64) float64 {
	f0 := float64(p0)
	f1 := float64(p1)
	f2 := float64(p2)
	f3 := float64(p3)
	t2 := t * t
	t3 := t2 * t
	return 0.5 * (2*f1 +
		(-f0+f2)*t +
		(2*f0-5*f1+4*f2-f3)*t2 +
		(-f0+3*f1-3*f2+f3)*t3)
}

func clampInt16(v float64) int16 {
	if v >= 0 {
		v += 0.5
	} else {
		v -= 0.5
	}
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}

// reset clears history for a new session / gen. The lifetime inject/drop totals
// survive (the API exposes a running total).
func (r *resampler) reset() {
	for ch := 0; ch < stream.Channels; ch++ {
		r.carry[ch] = r.carry[ch][:0]
	}
	r.primed = false
}
