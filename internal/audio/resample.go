package audio

import "ondaire/internal/stream"

// resampler does linear interpolation from inRate to 48000 on interleaved
// stereo int16 (it runs AFTER mono→stereo duplication, so it is always
// 2-channel). Pass-through when inRate == 48000. It keeps the last input
// sample-frame across calls so block boundaries interpolate seamlessly (no
// clicks at 20 ms edges).
type resampler struct {
	inRate int
	step   int64 // 32.32 fixed-point input step per output frame
	pos    int64 // 32.32 fixed-point input position within the pending block
	lastL  int16
	lastR  int16
	primed bool
}

// newResampler builds a resampler from inRate to the canonical 48000 Hz.
func newResampler(inRate int) *resampler {
	return &resampler{
		inRate: inRate,
		step:   (int64(inRate) << 32) / int64(stream.SampleRate),
	}
}

// process consumes interleaved-stereo int16 input and appends interleaved-
// stereo int16 output (at 48000) to out, returning the grown slice. atEOF
// flushes the tail (blends the final input frame against its own duplicate).
func (r *resampler) process(in []int16, atEOF bool, out []int16) []int16 {
	if r.inRate == stream.SampleRate {
		// Pass-through, bit-exact.
		return append(out, in...)
	}

	// Build the working buffer: the carried last frame (if primed) followed by
	// the new input. pos is a cursor into this combined buffer in 32.32
	// fixed-point input-frame units, where frame 0 is the carried frame.
	nIn := len(in) / 2
	if nIn == 0 && !atEOF {
		return out
	}

	// total input frames available for interpolation in this pass.
	var lead int
	if r.primed {
		lead = 1
	}
	total := lead + nIn

	get := func(i int) (int16, int16) {
		if i < lead {
			return r.lastL, r.lastR
		}
		j := (i - lead) * 2
		return in[j], in[j+1]
	}

	// We can emit an output frame as long as the upper neighbour (i+1) exists.
	// limit is the position (32.32) just past the last interpolatable input
	// frame. When atEOF we duplicate the final frame so the tail is emitted.
	maxBase := total - 1
	if atEOF {
		maxBase = total // allow i == total-1 with i+1 duplicated
	}

	for {
		i := int(r.pos >> 32)
		if i >= maxBase {
			break
		}
		frac := r.pos & 0xffffffff
		l0, rr0 := get(i)
		var l1, rr1 int16
		if i+1 < total {
			l1, rr1 = get(i + 1)
		} else {
			l1, rr1 = l0, rr0 // EOF tail duplicate
		}
		out = append(out,
			lerp(l0, l1, frac),
			lerp(rr0, rr1, frac),
		)
		r.pos += r.step
	}

	if atEOF {
		// Session flushed: drop carry, reset cursor.
		r.primed = false
		r.pos = 0
		return out
	}

	// Carry the lower-neighbor frame the next block will need (the index the
	// loop stopped at) as the new frame 0, and rebase pos relative to it.
	base := int(r.pos >> 32) // == total-1 (the last frame, no upper neighbor yet)
	if base > total-1 {
		base = total - 1
	}
	if base < 0 {
		base = 0
	}
	if total > 0 {
		r.lastL, r.lastR = get(base)
		r.primed = true
		r.pos -= int64(base) << 32
	}
	return out
}

// lerp linearly interpolates between a and b by frac (32.32 fractional part,
// 0..2^32) with rounding, clamped to int16.
func lerp(a, b int16, frac int64) int16 {
	d := int64(b) - int64(a)
	v := int64(a) + ((d*frac + (1 << 31)) >> 32)
	if v > 32767 {
		v = 32767
	} else if v < -32768 {
		v = -32768
	}
	return int16(v)
}
