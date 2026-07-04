package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"ondaire/internal/stream"
)

func TestFileTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, uri := range []string{"file:../x.wav", "file:../../etc/passwd", "/etc/passwd"} {
		if _, err := Open(ctx, uri, dir); !errors.Is(err, ErrTraversal) {
			t.Fatalf("%q err = %v, want ErrTraversal", uri, err)
		}
	}
}

func TestFileMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(context.Background(), "file:gone.wav", dir)
	if !errors.Is(err, ErrBadMedia) {
		t.Fatalf("missing err = %v, want ErrBadMedia", err)
	}
}

func TestFilePullPacedFrames(t *testing.T) {
	dir := t.TempDir()
	// Exactly 2 frames of 48k stereo audio (no padding).
	in := genTone(48000, 2, 1000, 960*2)
	if err := os.WriteFile(filepath.Join(dir, "a.wav"), writeWAVs16(48000, 2, in), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := Open(context.Background(), "a.wav", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	buf := make([]byte, stream.FrameBytes)
	var got []int16
	for {
		err := src.ReadFrame(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < stream.FrameSamples*stream.Channels; i++ {
			got = append(got, int16(binary.LittleEndian.Uint16(buf[i*2:])))
		}
	}
	if len(got) != len(in) {
		t.Fatalf("got %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("sample %d: %d != %d (resample drift on 48k passthrough)", i, got[i], in[i])
		}
	}
}
