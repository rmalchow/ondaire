package audio

import (
	"testing"

	"ondaire/internal/stream"
)

func TestResamplePassthrough48k(t *testing.T) {
	r := newResampler(stream.SampleRate)
	in := []int16{1, 2, 3, 4, 5, 6, 7, 8}
	out := r.process(in, true, nil)
	if len(out) != len(in) {
		t.Fatalf("passthrough length = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("passthrough not bit-exact at %d: %d != %d", i, out[i], in[i])
		}
	}
}

func TestResampleHalfRate(t *testing.T) {
	// 24000 → 48000 doubles the frame count (±1).
	r := newResampler(24000)
	nIn := 1000
	in := make([]int16, nIn*2)
	for i := 0; i < nIn; i++ {
		in[i*2] = int16(i % 200)
		in[i*2+1] = int16(i % 200)
	}
	out := r.process(in, true, nil)
	got := len(out) / 2
	want := nIn * 2
	if got < want-2 || got > want+2 {
		t.Fatalf("halfrate frames = %d, want ~%d", got, want)
	}
}

func TestResampleUpDownRatios(t *testing.T) {
	for _, rate := range []int{44100, 96000, 22050} {
		r := newResampler(rate)
		nIn := 2000
		in := make([]int16, nIn*2)
		for i := range in {
			in[i] = int16(i % 50)
		}
		out := r.process(in, true, nil)
		got := len(out) / 2
		want := nIn * stream.SampleRate / rate
		if got < want-3 || got > want+3 {
			t.Fatalf("rate %d: frames %d, want ~%d", rate, got, want)
		}
	}
}

func TestResampleBlockBoundaryContinuity(t *testing.T) {
	mk := func() []int16 {
		s := make([]int16, 400)
		for i := range s {
			s[i] = int16((i * 37) % 1000)
		}
		return s
	}
	full := mk()
	r1 := newResampler(44100)
	one := r1.process(full, true, nil)

	r2 := newResampler(44100)
	two := r2.process(full[:200], false, nil)
	two = r2.process(full[200:], true, two)

	if len(one) != len(two) {
		t.Fatalf("split length %d != single %d", len(two), len(one))
	}
	for i := range one {
		if one[i] != two[i] {
			t.Fatalf("split differs at %d: %d != %d", i, two[i], one[i])
		}
	}
}

func TestResampleConstantSignal(t *testing.T) {
	r := newResampler(44100)
	in := make([]int16, 800)
	for i := range in {
		in[i] = 5000 // DC
	}
	out := r.process(in, true, nil)
	for i, v := range out {
		if v < 4990 || v > 5010 {
			t.Fatalf("DC overshoot at %d: %d", i, v)
		}
	}
}
