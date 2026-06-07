package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"ensemble/internal/stream"
)

func TestMonoDuplicated(t *testing.T) {
	// Mono 48k WAV → framer must duplicate L==R.
	mono := []int16{100, 200, 300, 400}
	w := writeWAVs16(48000, 1, mono)
	dec, err := newDecoder(bytes.NewReader(w), "wav")
	if err != nil {
		t.Fatal(err)
	}
	fr := newFramer(dec)
	frame := make([]byte, stream.FrameBytes)
	if err := fr.frame(frame); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(mono); i++ {
		l := int16(binary.LittleEndian.Uint16(frame[i*4:]))
		r := int16(binary.LittleEndian.Uint16(frame[i*4+2:]))
		if l != r || l != mono[i] {
			t.Fatalf("sample %d: L=%d R=%d, want %d", i, l, r, mono[i])
		}
	}
}

func TestFinalFramePadded(t *testing.T) {
	// 48k stereo, not a multiple of 960 frames: last frame zero-padded, nil err,
	// then io.EOF (D9).
	in := genTone(48000, 2, 440, 960+10) // one full frame + 10 sample-frames
	w := writeWAVs16(48000, 2, in)
	dec, err := newDecoder(bytes.NewReader(w), "wav")
	if err != nil {
		t.Fatal(err)
	}
	fr := newFramer(dec)
	buf := make([]byte, stream.FrameBytes)

	if err := fr.frame(buf); err != nil {
		t.Fatalf("frame 0: %v", err)
	}
	if err := fr.frame(buf); err != nil {
		t.Fatalf("frame 1 (partial): %v", err)
	}
	// frame 1 holds 10 sample-frames of audio then zero padding.
	if !isSilent(buf[10*4:]) {
		t.Fatalf("padding not zero")
	}
	if err := fr.frame(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("frame 2: %v, want io.EOF", err)
	}
}

func TestEmptyImmediateEOF(t *testing.T) {
	w := writeWAVs16(48000, 2, nil)
	dec, err := newDecoder(bytes.NewReader(w), "wav")
	if err != nil {
		t.Fatal(err)
	}
	fr := newFramer(dec)
	buf := make([]byte, stream.FrameBytes)
	if err := fr.frame(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("empty source: %v, want io.EOF", err)
	}
}

func TestSniffDispatch(t *testing.T) {
	w := writeWAVs16(48000, 2, []int16{1, 2, 3, 4})
	dec, err := newDecoder(bytes.NewReader(w), "") // empty format → sniff
	if err != nil {
		t.Fatalf("sniff wav: %v", err)
	}
	if _, ok := dec.(*wavSource); !ok {
		t.Fatalf("sniff did not pick wavSource: %T", dec)
	}

	_, err = newDecoder(bytes.NewReader([]byte("not audio at all xxxx")), "")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("unknown sniff err = %v, want ErrUnsupportedFormat", err)
	}
}
