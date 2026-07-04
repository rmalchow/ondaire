package sink

import (
	"encoding/binary"
	"testing"

	"ondaire/internal/stream"
)

func dcFrame(v int16) []byte {
	f := make([]byte, stream.FrameBytes)
	for i := 0; i < stream.FrameSamples*stream.Channels; i++ {
		binary.LittleEndian.PutUint16(f[i*2:i*2+2], uint16(v))
	}
	return f
}

func gainAt(frame []byte, sampleTime, ch int) int16 {
	off := (sampleTime*stream.Channels + ch) * stream.BytesPerSmpl
	return int16(binary.LittleEndian.Uint16(frame[off : off+2]))
}

func TestGainRampNoStepDiscontinuity(t *testing.T) {
	g := newGainStage(1.0)
	g.setTarget(0.5)
	const v = 10000
	frame := dcFrame(v)
	g.apply(frame)
	// The applied gain factor = out/v. It must move monotonically from 1.0 toward
	// 0.5 with bounded per-sample-time step.
	maxStep := (1.0 - 0.5) / float64(stream.FrameSamples)
	prev := 1.0
	for st := 0; st < stream.FrameSamples; st++ {
		out := gainAt(frame, st, 0)
		factor := float64(out) / float64(v)
		if factor > prev+1e-9 {
			t.Fatalf("sample-time %d: gain rose (%.5f > %.5f)", st, factor, prev)
		}
		if prev-factor > maxStep+0.01 {
			t.Fatalf("sample-time %d: step %.6f exceeds bound %.6f", st, prev-factor, maxStep)
		}
		prev = factor
	}
}

func TestGainHalvesAfterSettle(t *testing.T) {
	g := newGainStage(1.0)
	g.setTarget(0.5)
	const v = 8000
	// First frame settles current → 0.5.
	g.apply(dcFrame(v))
	// Second full-scale frame: every sample halved.
	frame := dcFrame(v)
	g.apply(frame)
	for st := 0; st < stream.FrameSamples; st++ {
		for ch := 0; ch < stream.Channels; ch++ {
			out := gainAt(frame, st, ch)
			if out != v/2 {
				t.Fatalf("sample-time %d ch %d: got %d, want %d", st, ch, out, v/2)
			}
		}
	}
}

func TestGainUnityIdentity(t *testing.T) {
	g := newGainStage(1.0)
	in := dcFrame(12345)
	frame := dcFrame(12345)
	g.apply(frame)
	for i := range frame {
		if frame[i] != in[i] {
			t.Fatalf("byte %d changed under unity gain", i)
		}
	}
}

func TestGainClampsRange(t *testing.T) {
	g := newGainStage(0.5)
	g.setTarget(2.0)
	g.apply(dcFrame(100)) // settle toward clamp
	g.apply(dcFrame(100))
	frame := dcFrame(100)
	g.apply(frame)
	if gainAt(frame, 10, 0) != 100 {
		t.Fatalf("target 2.0 should clamp to 1.0 (identity), got %d", gainAt(frame, 10, 0))
	}
	g.setTarget(-1)
	g.apply(dcFrame(100))
	g.apply(dcFrame(100))
	frame = dcFrame(100)
	g.apply(frame)
	if gainAt(frame, 10, 0) != 0 {
		t.Fatalf("target -1 should clamp to 0.0 (mute), got %d", gainAt(frame, 10, 0))
	}
}

func TestGainStereoSymmetric(t *testing.T) {
	g := newGainStage(1.0)
	g.setTarget(0.5)
	// Distinct L/R values so we can verify both get the same factor per sample-time.
	frame := make([]byte, stream.FrameBytes)
	for st := 0; st < stream.FrameSamples; st++ {
		off := st * stream.Channels * stream.BytesPerSmpl
		binary.LittleEndian.PutUint16(frame[off:off+2], uint16(int16(1000)))
		binary.LittleEndian.PutUint16(frame[off+2:off+4], uint16(int16(2000)))
	}
	g.apply(frame)
	for st := 0; st < stream.FrameSamples; st++ {
		l := gainAt(frame, st, 0)
		r := gainAt(frame, st, 1)
		// r should be ~2x l (same factor applied to inputs 1000 and 2000).
		if l == 0 {
			continue
		}
		ratio := float64(r) / float64(l)
		if ratio < 1.9 || ratio > 2.1 {
			t.Fatalf("sample-time %d: L=%d R=%d ratio %.3f != ~2", st, l, r, ratio)
		}
	}
}
