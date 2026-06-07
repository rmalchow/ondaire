package audio

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"ensemble/internal/dl"
	"ensemble/internal/stream"
)

// canonSineFrame builds one canonical frame (3840 B) of a stereo sine at freq.
func canonSineFrame(freq float64, phase int) []byte {
	buf := make([]byte, stream.FrameBytes)
	for i := 0; i < stream.FrameSamples; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(phase+i)/float64(stream.SampleRate)) * 12000)
		binary.LittleEndian.PutUint16(buf[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(buf[i*4+2:], uint16(v))
	}
	return buf
}

func frameRMS(b []byte) float64 {
	var sum float64
	n := stream.FrameSamples * stream.Channels
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(b[i*2:]))
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(n))
}

func TestOpusRoundTrip(t *testing.T) {
	enc, err := NewOpusEncoder()
	if errors.Is(err, dl.ErrUnavailable) {
		t.Skip("libopus not loadable on this host")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	dec, err := NewOpusDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	const frames = 50 // 1 s
	for f := 0; f < frames; f++ {
		in := canonSineFrame(440, f*stream.FrameSamples)
		pkt, err := enc.Encode(in)
		if err != nil {
			t.Fatalf("encode %d: %v", f, err)
		}
		if len(pkt) == 0 || len(pkt) > opusMaxPacket {
			t.Fatalf("encode %d: bad packet len %d", f, len(pkt))
		}
		out, err := dec.Decode(pkt)
		if err != nil {
			t.Fatalf("decode %d: %v", f, err)
		}
		if len(out) != stream.FrameBytes {
			t.Fatalf("decode %d: len %d, want %d", f, len(out), stream.FrameBytes)
		}
		// Lossy: compare RMS energy (skip the first frame — encoder warm-up).
		if f > 2 {
			inR, outR := frameRMS(in), frameRMS(out)
			if math.Abs(inR-outR) > inR*0.5+500 {
				t.Fatalf("frame %d: RMS in=%.0f out=%.0f differ too much", f, inR, outR)
			}
		}
	}
}

func TestOpusUnavailable(t *testing.T) {
	// Deterministically exercise the ErrUnavailable path via a bogus soname,
	// using the same dl.Open shape the constructors use.
	_, err := dl.Open([]string{"libopus-nope.so.0", "libopus-nope.so"}, opusSymbols)
	if !errors.Is(err, dl.ErrUnavailable) {
		t.Fatalf("bogus soname err = %v, want dl.ErrUnavailable", err)
	}
}

func TestOpusAvailableMatchesConstructor(t *testing.T) {
	avail := OpusAvailable()
	enc, err := NewOpusEncoder()
	if err == nil {
		enc.Close()
	}
	constructorOK := err == nil
	if avail != constructorOK {
		t.Fatalf("OpusAvailable()=%v but NewOpusEncoder ok=%v (err=%v)", avail, constructorOK, err)
	}
}

func TestOpusCloseIdempotent(t *testing.T) {
	enc, err := NewOpusEncoder()
	if errors.Is(err, dl.ErrUnavailable) {
		t.Skip("libopus not loadable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("enc close 1: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("enc close 2: %v", err)
	}

	dec, err := NewOpusDecoder()
	if err != nil {
		t.Fatal(err)
	}
	dec.Close()
	dec.Close()
}
