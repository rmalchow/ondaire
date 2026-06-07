package audio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ensemble/internal/stream"
)

// withFakeCapture installs the fake capture binary as "pw-record" on PATH and
// optionally sets env for the helper.
func withFakeCapture(t *testing.T, env ...string) {
	t.Helper()
	bin := fakeCaptureExe(t)
	dir := t.TempDir()
	link := filepath.Join(dir, "pw-record")
	if err := os.Symlink(bin, link); err != nil {
		// Fall back to copy if symlink unsupported.
		data, _ := os.ReadFile(bin)
		os.WriteFile(link, data, 0o755)
	}
	t.Setenv("PATH", dir)
	for _, e := range env {
		i := 0
		for ; i < len(e); i++ {
			if e[i] == '=' {
				break
			}
		}
		t.Setenv(e[:i], e[i+1:])
	}
}

func TestInputFakeCaptureFrames(t *testing.T) {
	withFakeCapture(t, "FAKE_FRAMES=4000")
	src, err := Open(context.Background(), "input:", t.TempDir())
	if err != nil {
		t.Fatalf("open input: %v", err)
	}
	defer src.Close()
	if !src.Live() {
		t.Fatalf("input source not Live()")
	}
	buf := make([]byte, stream.FrameBytes)
	// At least one frame should carry the (non-silent) tone.
	sawAudio := false
	for i := 0; i < 5; i++ {
		if err := src.ReadFrame(buf); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if !isSilent(buf) {
			sawAudio = true
		}
	}
	if !sawAudio {
		t.Fatalf("no audio frames from fake capture")
	}
}

func TestInputNoBackendIsBadMedia(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := Open(context.Background(), "input:", t.TempDir())
	if !errors.Is(err, ErrBadMedia) {
		t.Fatalf("no backend err = %v, want ErrBadMedia", err)
	}
}

func TestInputCloseKillsProcess(t *testing.T) {
	withFakeCapture(t, "FAKE_FRAMES=480", "FAKE_STALL=1")
	src, err := Open(context.Background(), "input:", t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buf := make([]byte, stream.FrameBytes)
	src.ReadFrame(buf)

	done := make(chan struct{})
	go func() { src.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("Close did not kill the stalled capture process promptly")
	}
}

func TestInputStallSilence(t *testing.T) {
	// Helper emits a short burst then stalls forever; later ReadFrames yield
	// silence without blocking.
	withFakeCapture(t, "FAKE_FRAMES=480", "FAKE_STALL=1")
	src, err := Open(context.Background(), "input:", t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	buf := make([]byte, stream.FrameBytes)
	// Drain the burst (~1 frame) then expect a prompt silence frame.
	for i := 0; i < 3; i++ {
		src.ReadFrame(buf)
	}
	start := time.Now()
	if err := src.ReadFrame(buf); err != nil {
		t.Fatalf("stall read: %v", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("stall read blocked %v", time.Since(start))
	}
}
