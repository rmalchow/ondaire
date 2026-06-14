package sink

import (
	"encoding/binary"

	"ensemble/internal/stream"
)

// resampler is a PULL-based 4-tap Catmull-Rom resampler (PLAN-dac-pull-phase-lock).
// The caller feeds whole input frames (feed) and pulls a fixed FrameSamples of output
// per process() call at a given ratio (input samples advanced per output sample, ≈1).
// Output is fixed-size because the ALSA backend requires exactly FrameBytes writes; the
// rate correction lives entirely in the ratio (how fast the cursor advances through the
// input), which is the servo's actuator. Run independently per channel (interleaved L/R).
//
// Catmull-Rom for fractional position t∈[0,1) between samples p1 and p2, neighbours p0,p3:
//
//	y(t) = 0.5*( 2*p1 + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t^2 + (-p0+3*p1-3*p2+p3)*t^3 )
//
// in[ch] holds [leadPad lookback history][fed-but-unconsumed input]; pos is the float read
// cursor (≥ leadPad). Each process advances pos by FrameSamples*ratio, then drops the whole
// consumed samples from the front (keeping leadPad of lookback) so the buffer stays bounded.
type resampler struct {
	in     [stream.Channels][]int32 // input buffer; in[ch][0:leadPad] is lookback history
	pos    float64                  // read cursor within in (>= leadPad)
	primed bool

	out []byte

	// consumed is the cumulative count of input samples the cursor has advanced past
	// since reset (fractional). fedPTS = originPTS + consumed*nsPerSample — the master
	// time of the read cursor, the play-head reference for the phase-lock servo.
	consumed float64
	// Realized rate-match accounting (per-channel samples, = time): surplus/deficit of
	// input consumed vs the nominal one-frame-per-output. ratio>1 (compress, catching up)
	// consumes extra → dropped; ratio<1 (stretch) consumes fewer → injected.
	injected float64
	dropped  float64
}

const (
	// leadPad: leading lookback samples kept so p0 (pos-1) is valid at the seam.
	leadPad = 3
	// lookahead: Catmull-Rom needs p2,p3 ahead of the cursor.
	lookahead = 2
	// needInput: samples the caller must keep available ahead of the cursor before
	// process() — one output frame at the max ratio plus the lookahead taps.
	needInput = stream.FrameSamples + 8 + lookahead + 1
)

func newResampler() *resampler {
	r := &resampler{out: make([]byte, stream.FrameBytes)}
	for ch := range r.in {
		r.in[ch] = make([]int32, 0, 3*stream.FrameSamples)
	}
	return r
}

// feed appends one input frame (exactly FrameBytes). On the first feed it seeds leadPad
// lookback (held = the frame's first sample) so the very first seam has a real 4-tap window.
func (r *resampler) feed(frame []byte) {
	const n = stream.FrameSamples
	if !r.primed {
		for ch := 0; ch < stream.Channels; ch++ {
			r.in[ch] = r.in[ch][:0]
			s0 := int32(int16(binary.LittleEndian.Uint16(frame[ch*stream.BytesPerSmpl : ch*stream.BytesPerSmpl+2])))
			for j := 0; j < leadPad; j++ {
				r.in[ch] = append(r.in[ch], s0)
			}
		}
		r.pos = leadPad
		r.primed = true
	}
	for i := 0; i < n; i++ {
		base := i * stream.Channels * stream.BytesPerSmpl
		for ch := 0; ch < stream.Channels; ch++ {
			off := base + ch*stream.BytesPerSmpl
			r.in[ch] = append(r.in[ch], int32(int16(binary.LittleEndian.Uint16(frame[off:off+2]))))
		}
	}
}

// inputAvail returns how many input samples are available ahead of the cursor (incl. taps).
func (r *resampler) inputAvail() int {
	if !r.primed {
		return 0
	}
	return len(r.in[0]) - int(r.pos)
}

// consumedSamples returns the cumulative input samples the cursor has advanced (for fedPTS).
func (r *resampler) consumedSamples() float64 { return r.consumed }

// process produces exactly FrameSamples output samples at the given ratio. The caller must
// ensure inputAvail() >= needInput first. Returns the FrameBytes output slice.
func (r *resampler) process(ratio float64) []byte {
	const n = stream.FrameSamples
	for ch := 0; ch < stream.Channels; ch++ {
		buf := r.in[ch]
		last := len(buf) - 1
		for k := 0; k < n; k++ {
			p := r.pos + float64(k)*ratio
			idx := int(p)
			t := p - float64(idx)
			y := catmullRom(atIdx(buf, idx-1, last), atIdx(buf, idx, last),
				atIdx(buf, idx+1, last), atIdx(buf, idx+2, last), t)
			v := clampInt16(y)
			off := (k*stream.Channels + ch) * stream.BytesPerSmpl
			binary.LittleEndian.PutUint16(r.out[off:off+2], uint16(v))
		}
	}
	adv := float64(n) * ratio
	r.consumed += adv
	r.pos += adv
	if d := adv - float64(n); d > 0 {
		r.dropped += d // consumed more input than output (compressing)
	} else if d < 0 {
		r.injected += -d // consumed less (stretching)
	}
	// Drop whole consumed samples from the front, keep leadPad of lookback before the cursor.
	drop := int(r.pos) - leadPad
	if drop > 0 {
		for ch := 0; ch < stream.Channels; ch++ {
			buf := r.in[ch]
			if drop > len(buf) {
				drop = len(buf)
			}
			copy(buf, buf[drop:])
			r.in[ch] = buf[:len(buf)-drop]
		}
		r.pos -= float64(drop)
	}
	return r.out
}

// sampleStats returns the cumulative realized rate-match counts (per-channel samples).
func (r *resampler) sampleStats() (injected, dropped uint64) {
	return uint64(r.injected), uint64(r.dropped)
}

// atIdx returns buf[i], clamping to the buffer ends (the live edge / lookahead tap).
func atIdx(buf []int32, i, last int) int32 {
	if i < 0 {
		i = 0
	} else if i > last {
		i = last
	}
	return buf[i]
}

func catmullRom(p0, p1, p2, p3 int32, t float64) float64 {
	f0, f1, f2, f3 := float64(p0), float64(p1), float64(p2), float64(p3)
	t2 := t * t
	t3 := t2 * t
	return 0.5 * (2*f1 + (-f0+f2)*t + (2*f0-5*f1+4*f2-f3)*t2 + (-f0+3*f1-3*f2+f3)*t3)
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

// reset clears history for a new session / gen. Lifetime inject/drop totals survive.
func (r *resampler) reset() {
	for ch := 0; ch < stream.Channels; ch++ {
		r.in[ch] = r.in[ch][:0]
	}
	r.pos = 0
	r.consumed = 0 // per-session read cursor; the engine anchors fedPTS to it
	r.primed = false
}
