package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"ondaire/internal/stream"
)

// A 2 s, 48 kHz stereo WAV whose first second is +1000 and second is -1000.
// Seeking to 1.5 s must land in the second (negative) half. 48 kHz == canonical,
// so the framer passes samples through without resampling.
func TestWAVSeekLandsInSecondHalf(t *testing.T) {
	dir := t.TempDir()
	const rate, ch = 48000, 2
	samples := make([]int16, 0, rate*ch*2)
	for i := 0; i < rate*ch; i++ {
		samples = append(samples, 1000)
	}
	for i := 0; i < rate*ch; i++ {
		samples = append(samples, -1000)
	}
	if err := os.WriteFile(filepath.Join(dir, "ramp.wav"), writeWAVs16(rate, ch, samples), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := Open(context.Background(), "file:ramp.wav", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	sk, ok := src.(Seeker)
	if !ok {
		t.Fatal("file source should implement Seeker")
	}
	if err := sk.Seek(1.5); err != nil {
		t.Fatalf("seek: %v", err)
	}
	buf := make([]byte, stream.FrameBytes)
	if err := src.ReadFrame(buf); err != nil {
		t.Fatalf("read after seek: %v", err)
	}
	if v := int16(binary.LittleEndian.Uint16(buf[0:2])); v > -500 {
		t.Fatalf("after seek to 1.5s sample = %d, want ~ -1000 (second half)", v)
	}
}

func TestFixtureDecoderSeek(t *testing.T) {
	for _, tc := range []struct{ file, format string }{
		{"tone.mp3", "mp3"},
		{"tone.flac", "flac"},
	} {
		p, skip := maybeFixture(t, tc.file)
		if skip {
			t.Skipf("no testdata/%s fixture", tc.file)
		}
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := newDecoder(f, tc.format)
		if err != nil {
			f.Close()
			t.Fatalf("%s decode: %v", tc.file, err)
		}
		sk, ok := dec.(decoderSeeker)
		if !ok {
			f.Close()
			t.Fatalf("%s decoder is not seekable", tc.format)
		}
		if err := sk.seek(0.25); err != nil { // within both fixtures (~0.5s long)
			f.Close()
			t.Fatalf("%s seek: %v", tc.file, err)
		}
		out, err := dec.read(nil)
		f.Close()
		if err != nil && err != io.EOF {
			t.Fatalf("%s read after seek: %v", tc.file, err)
		}
		if len(out) == 0 && err != io.EOF {
			t.Fatalf("%s: no samples after seek", tc.file)
		}
	}
}

// A non-seekable reader (Seeker hidden) must report ErrNotSeekable rather than
// silently misbehaving — this is what marks http/stream sources non-seekable.
func TestNonSeekableDecoderReportsError(t *testing.T) {
	p, skip := maybeFixture(t, "tone.flac")
	if skip {
		t.Skip("no testdata/tone.flac fixture")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Wrap so only io.Reader is exposed (not io.Seeker).
	dec, err := newDecoder(struct{ io.Reader }{bytes.NewReader(data)}, "flac")
	if err != nil {
		t.Fatal(err)
	}
	sk, ok := dec.(decoderSeeker)
	if !ok {
		t.Fatal("flac decoder should expose seek()")
	}
	if err := sk.seek(0.5); !errors.Is(err, ErrNotSeekable) {
		t.Fatalf("seek on non-seekable reader = %v, want ErrNotSeekable", err)
	}
}
