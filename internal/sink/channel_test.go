package sink

import (
	"encoding/binary"
	"testing"

	"ensemble/internal/stream"
)

// buildStereoFrame makes a FrameBytes stereo frame with L sample-times = lv and
// R = rv (constant), for an easy dual-mono assertion.
func buildStereoFrame(lv, rv int16) []byte {
	f := make([]byte, stream.FrameBytes)
	for i := 0; i < stream.FrameSamples; i++ {
		off := i * stream.Channels * stream.BytesPerSmpl
		binary.LittleEndian.PutUint16(f[off:off+2], uint16(lv))
		binary.LittleEndian.PutUint16(f[off+2:off+4], uint16(rv))
	}
	return f
}

func chans(frame []byte) (l, r int16) {
	l = int16(binary.LittleEndian.Uint16(frame[0:2]))
	r = int16(binary.LittleEndian.Uint16(frame[2:4]))
	return
}

func TestChannelStageDualMono(t *testing.T) {
	cases := []struct {
		mode         string
		wantL, wantR int16
	}{
		{"stereo", 100, -200}, // untouched
		{"L", 100, 100},       // left copied over right
		{"R", -200, -200},     // right copied over left
	}
	for _, tc := range cases {
		cs := newChannelStage(tc.mode)
		f := buildStereoFrame(100, -200)
		cs.apply(f)
		// check several sample-times, not just the first
		for _, i := range []int{0, 1, stream.FrameSamples / 2, stream.FrameSamples - 1} {
			off := i * stream.Channels * stream.BytesPerSmpl
			l := int16(binary.LittleEndian.Uint16(f[off : off+2]))
			r := int16(binary.LittleEndian.Uint16(f[off+2 : off+4]))
			if l != tc.wantL || r != tc.wantR {
				t.Fatalf("%s @i=%d: got (L=%d,R=%d), want (L=%d,R=%d)", tc.mode, i, l, r, tc.wantL, tc.wantR)
			}
		}
	}
}

func TestChannelStageStereoIsBitIdentical(t *testing.T) {
	cs := newChannelStage("stereo")
	f := buildStereoFrame(1234, -4321)
	orig := append([]byte(nil), f...)
	cs.apply(f)
	for i := range f {
		if f[i] != orig[i] {
			t.Fatalf("stereo mode altered byte %d: %d != %d", i, f[i], orig[i])
		}
	}
}

func TestChannelStageLiveSet(t *testing.T) {
	cs := newChannelStage("stereo")
	if cs.current() != chStereo {
		t.Fatal("initial mode not stereo")
	}
	cs.set("R")
	f := buildStereoFrame(7, 9)
	cs.apply(f)
	if l, r := chans(f); l != 9 || r != 9 {
		t.Fatalf("after set(R): got (L=%d,R=%d), want (9,9)", l, r)
	}
}
