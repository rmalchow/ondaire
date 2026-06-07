package audio

import (
	"bytes"
	"errors"
	"testing"
)

func readAll(t *testing.T, sr sampleReader) []int16 {
	t.Helper()
	var all []int16
	for {
		var err error
		all, err = sr.read(all)
		if err != nil {
			return all
		}
	}
}

func TestWAVParseS16(t *testing.T) {
	in := genTone(48000, 2, 440, 1000)
	w := writeWAVs16(48000, 2, in)
	sr, err := newWAVSource(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	rate, ch := sr.info()
	if rate != 48000 || ch != 2 {
		t.Fatalf("info = %d/%d", rate, ch)
	}
	got := readAll(t, sr)
	if len(got) != len(in) {
		t.Fatalf("got %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("sample %d: %d != %d", i, got[i], in[i])
		}
	}
}

func TestWAVParseU8(t *testing.T) {
	// 0x80 (128) → 0; 0xFF → near +full; 0x00 → near -full.
	w := writeWAVu8(8000, 1, []uint8{0x80, 0xFF, 0x00, 0x80})
	sr, err := newWAVSource(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, sr)
	if got[0] != 0 {
		t.Fatalf("0x80 → %d, want 0", got[0])
	}
	if got[1] <= 0 {
		t.Fatalf("0xFF → %d, want positive", got[1])
	}
	if got[2] >= 0 {
		t.Fatalf("0x00 → %d, want negative", got[2])
	}
}

func TestWAVParseS24(t *testing.T) {
	// A positive and a negative 24-bit value; verify sign preserved.
	w := writeWAVs24(8000, 1, []int32{0x400000, -0x400000})
	sr, err := newWAVSource(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, sr)
	if got[0] <= 0 || got[1] >= 0 {
		t.Fatalf("s24 sign not preserved: %d, %d", got[0], got[1])
	}
}

func TestWAVParseFloat32(t *testing.T) {
	w := writeWAVfloat32(8000, 1, []float32{0, 1.0, -1.0, 2.0, -2.0})
	sr, err := newWAVSource(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, sr)
	if got[0] != 0 {
		t.Fatalf("0.0 → %d", got[0])
	}
	if got[3] != 32767 || got[4] != -32768 {
		t.Fatalf("float clamp failed: %d, %d", got[3], got[4])
	}
}

func TestWAVSkipsAuxChunks(t *testing.T) {
	// Build a WAV with a LIST chunk before data.
	base := writeWAVs16(48000, 1, []int16{10, 20, 30})
	// Surgery: insert a LIST chunk right after "WAVE". Easier: rebuild.
	b := new(bytes.Buffer)
	data := []byte{10, 0, 20, 0, 30, 0}
	list := []byte("INFOxxxx")
	b.WriteString("RIFF")
	writeU32(b, uint32(4+(8+16)+(8+len(list))+(8+len(data))))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	writeU32(b, 16)
	writeU16(b, uint16(wavPCM))
	writeU16(b, 1)
	writeU32(b, 48000)
	writeU32(b, 48000*2)
	writeU16(b, 2)
	writeU16(b, 16)
	b.WriteString("LIST")
	writeU32(b, uint32(len(list)))
	b.Write(list)
	b.WriteString("data")
	writeU32(b, uint32(len(data)))
	b.Write(data)

	_ = base
	sr, err := newWAVSource(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, sr)
	if len(got) != 3 || got[0] != 10 || got[2] != 30 {
		t.Fatalf("aux-chunk skip wrong: %v", got)
	}
}

func TestWAVTruncatedDataIsEOF(t *testing.T) {
	w := writeWAVs16(48000, 2, genTone(48000, 2, 440, 100))
	// Chop off a couple of bytes mid-frame.
	sr, err := newWAVSource(bytes.NewReader(w[:len(w)-3]))
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, sr) // must not panic; drops the partial frame
	if len(got)%2 != 0 {
		t.Fatalf("odd sample count %d", len(got))
	}
}

func TestWAVRejectsALaw(t *testing.T) {
	w := writeWAVs16(48000, 1, []int16{1, 2})
	// Patch the format tag (offset 20) to A-law (6).
	w[20] = 6
	_, err := newWAVSource(bytes.NewReader(w))
	if !errors.Is(err, ErrBadMedia) {
		t.Fatalf("a-law err = %v, want ErrBadMedia", err)
	}
}

func TestWAVRejectsMissingDataChunk(t *testing.T) {
	b := new(bytes.Buffer)
	b.WriteString("RIFF")
	writeU32(b, 4+8+16)
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	writeU32(b, 16)
	writeU16(b, uint16(wavPCM))
	writeU16(b, 1)
	writeU32(b, 48000)
	writeU32(b, 96000)
	writeU16(b, 2)
	writeU16(b, 16)
	_, err := newWAVSource(bytes.NewReader(b.Bytes()))
	if !errors.Is(err, ErrBadMedia) {
		t.Fatalf("missing data err = %v, want ErrBadMedia", err)
	}
}
