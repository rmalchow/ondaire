package sink

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ensemble/internal/dl"
	"ensemble/internal/stream"
)

func frame() []byte { return make([]byte, stream.FrameBytes) }

func TestNullBackendCounts(t *testing.T) {
	b := newNullBackend()
	for i := 0; i < 5; i++ {
		if err := b.Write(frame()); err != nil {
			t.Fatal(err)
		}
	}
	if b.Written() != 5 {
		t.Fatalf("written=%d, want 5", b.Written())
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNullBackendRejectsWrongSize(t *testing.T) {
	b := newNullBackend()
	if err := b.Write(make([]byte, 10)); err == nil {
		t.Fatal("wrong-size frame should error")
	}
}

func TestNullBackendPaceOff(t *testing.T) {
	b := newNullBackend()
	b.pace = false
	// Many writes should not block (pace off).
	for i := 0; i < 100; i++ {
		if err := b.Write(frame()); err != nil {
			t.Fatal(err)
		}
	}
	if b.Written() != 100 {
		t.Fatalf("written=%d, want 100", b.Written())
	}
}

func TestFileBackendAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.pcm")
	b, err := newFileBackend(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Write(frame()); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(frame()); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(2*stream.FrameBytes) {
		t.Fatalf("size=%d, want %d", fi.Size(), 2*stream.FrameBytes)
	}
}

func TestFileBackendBadPath(t *testing.T) {
	if _, err := newFileBackend("/no/such/dir/file.pcm"); err == nil {
		t.Fatal("bad path should error")
	}
	if _, err := newFileBackend(""); err == nil {
		t.Fatal("empty path should error")
	}
}

func TestExecBackendSkippedIfNoTool(t *testing.T) {
	if _, _, ok := lookExecTool(); !ok {
		t.Skip("no exec player tool on $PATH")
	}
	b, err := newExecBackend(slog.Default())
	if err != nil {
		t.Skipf("exec backend spawn failed (no usable device?): %v", err)
	}
	for i := 0; i < 3; i++ {
		_ = b.Write(frame()) // may error if the player rejects raw stdin; tolerate
	}
	if err := b.Close(); err != nil {
		t.Logf("exec close returned: %v", err) // player exit status varies
	}
}

func TestAlsaBackendSkippedIfNoLib(t *testing.T) {
	if _, err := dl.Open(alsaSonames, alsaSymbols); err != nil {
		t.Skip("libasound not loadable")
	}
	if !isRegistered("alsa") {
		t.Skip("alsa not registered (probe failed)")
	}
	b, err := newAlsaBackend("default", slog.Default())
	if err != nil {
		t.Skipf("snd_pcm_open failed (no usable PCM device): %v", err)
	}
	defer b.Close()
	for i := 0; i < 3; i++ {
		if err := b.Write(frame()); err != nil {
			t.Logf("alsa write %d: %v", i, err)
		}
	}
	if _, ok := b.DeviceDelay(); !ok {
		t.Log("DeviceDelay reported ok=false (acceptable for some devices)")
	}
}
