package audio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ondaire/internal/stream"
)

func TestHTTPContentTypeDispatch(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(body)
	}))
	defer srv.Close()

	src, err := Open(context.Background(), srv.URL+"/stream", t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	if !src.Live() {
		t.Fatalf("http source not Live()")
	}
	buf := make([]byte, stream.FrameBytes)
	if err := src.ReadFrame(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if isSilent(buf) {
		t.Fatalf("expected audio, got silence")
	}
}

func TestHTTPExtensionFallback(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No / wrong content type → fall back to URL .wav extension.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(body)
	}))
	defer srv.Close()

	src, err := Open(context.Background(), srv.URL+"/song.wav", t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	buf := make([]byte, stream.FrameBytes)
	if err := src.ReadFrame(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
}

func TestHTTPLivePacedSilenceOnStall(t *testing.T) {
	// Server sends a header then trickles nothing for a while.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		hdr := writeWAVs16(48000, 2, nil) // just the header, no data
		w.Write(hdr)
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	src, err := Open(context.Background(), srv.URL, t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()

	buf := make([]byte, stream.FrameBytes)
	start := time.Now()
	if err := src.ReadFrame(buf); err != nil {
		t.Fatalf("read under stall: %v", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("ReadFrame stalled %v, should yield silence ~one frame period", time.Since(start))
	}
	if !isSilent(buf) {
		t.Fatalf("stall should yield silence")
	}
}

func TestHTTPFiniteBodyGoesSilent(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960*2))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(body)
	}))
	defer srv.Close()

	src, err := Open(context.Background(), srv.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	buf := make([]byte, stream.FrameBytes)
	// Pull many frames: first couple have audio, then silence — never EOF.
	sawSilence := false
	for i := 0; i < 10; i++ {
		if err := src.ReadFrame(buf); err != nil {
			t.Fatalf("frame %d: %v, want nil (live never EOFs)", i, err)
		}
		if isSilent(buf) {
			sawSilence = true
		}
	}
	if !sawSilence {
		t.Fatalf("finite body never went silent")
	}
}

func TestHTTPNon2xxIsBadMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := Open(context.Background(), srv.URL, t.TempDir())
	if !errors.Is(err, ErrBadMedia) {
		t.Fatalf("404 err = %v, want ErrBadMedia", err)
	}
}

func TestHTTPCloseCancels(t *testing.T) {
	// Server holds the connection open and trickles data forever.
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(writeWAVs16(48000, 2, genTone(48000, 2, 440, 480)))
		w.(http.Flusher).Flush()
		<-stop
	}))
	defer srv.Close()
	defer close(stop)

	src, err := Open(context.Background(), srv.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, stream.FrameBytes)
	src.ReadFrame(buf)

	done := make(chan struct{})
	go func() { src.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Close did not return promptly")
	}
	// After Close, ReadFrame returns io.EOF.
	if err := src.ReadFrame(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("post-close read = %v, want io.EOF", err)
	}
}
