package audio

import (
	"context"
	"testing"

	"ondaire/internal/stream"
)

func s16(b []byte) int16 { return int16(uint16(b[0]) | uint16(b[1])<<8) }

// The click train must: open via the calib: scheme, be live, emit full canonical
// frames, keep L==R, and place clicks ONLY inside the click window of each period.
func TestCalibClickTrain(t *testing.T) {
	src, err := Open(context.Background(), "calib:click?hz=4&level=0.5", "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	if !src.Live() {
		t.Fatal("calib source must be live (never EOF)")
	}

	const hz = 4
	period := stream.SampleRate / hz         // 12000 samples
	click := stream.SampleRate * 2 / 1000    // ~2 ms = 96 samples
	frames := period/stream.FrameSamples + 2 // cover >1 full period
	buf := make([]byte, stream.FrameBytes)

	idx, firstClickEnergy := 0, 0
	for f := 0; f < frames; f++ {
		if err := src.ReadFrame(buf); err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		for i := 0; i < stream.FrameSamples; i++ {
			l, r := s16(buf[i*4:]), s16(buf[i*4+2:])
			if l != r {
				t.Fatalf("L/R differ at sample %d: %d != %d", idx, l, r)
			}
			phase := idx % period
			if l != 0 && phase >= click {
				t.Fatalf("nonzero sample %d at phase %d (outside %d-sample click)", idx, phase, click)
			}
			if phase < click && l != 0 {
				firstClickEnergy++
			}
			idx++
		}
	}
	if firstClickEnergy == 0 {
		t.Fatal("click window produced no signal")
	}
}

func TestCalibModes(t *testing.T) {
	if s, err := Open(context.Background(), "calib:noise?level=0.3", ""); err != nil {
		t.Fatalf("noise mode: %v", err)
	} else {
		_ = s.Close()
	}
	if _, err := Open(context.Background(), "calib:bogus", ""); err == nil {
		t.Fatal("unknown calib mode should error")
	}
}
