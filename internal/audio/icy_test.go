package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/stream"
)

func TestParseStreamTitle(t *testing.T) {
	blk := []byte("StreamTitle='Artist - Song';StreamUrl='http://x';\x00\x00")
	if got := parseStreamTitle(blk); got != "Artist - Song" {
		t.Fatalf("parseStreamTitle = %q", got)
	}
	if got := parseStreamTitle([]byte("no title here")); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// icyEncode interleaves an ICY metadata block after every metaint audio bytes;
// the first full interval carries title, later intervals carry an empty block
// (length 0), mirroring real SHOUTcast behavior.
func icyEncode(audio []byte, metaint int, title string) []byte {
	meta := "StreamTitle='" + title + "';"
	for len(meta)%16 != 0 {
		meta += "\x00"
	}
	block := append([]byte{byte(len(meta) / 16)}, meta...)
	var out []byte
	first := true
	for len(audio) > 0 {
		n := metaint
		if n > len(audio) {
			n = len(audio)
		}
		out = append(out, audio[:n]...)
		audio = audio[n:]
		if n == metaint {
			if first {
				out = append(out, block...)
				first = false
			} else {
				out = append(out, 0) // empty metadata block
			}
		}
	}
	return out
}

func TestHTTPICYMetadata(t *testing.T) {
	wav := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960*8))
	const metaint = 256
	body := icyEncode(wav, metaint, "DJ Cool - Night Drive")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Icy-Metadata") != "1" {
			t.Errorf("client did not request ICY metadata: %q", r.Header.Get("Icy-Metadata"))
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("icy-metaint", strconv.Itoa(metaint))
		w.Write(body)
	}))
	defer srv.Close()

	src, err := Open(context.Background(), srv.URL+"/stream", t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()

	md, ok := src.(interface {
		Metadata() (contracts.TrackMetadata, bool)
	})
	if !ok {
		t.Fatal("http source does not expose Metadata()")
	}

	buf := make([]byte, stream.FrameBytes)
	var got contracts.TrackMetadata
	for i := 0; i < 50; i++ {
		if err := src.ReadFrame(buf); err != nil {
			t.Fatalf("read: %v", err)
		}
		if m, has := md.Metadata(); has {
			got = m
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got.Artist != "DJ Cool" || got.Title != "Night Drive" {
		t.Fatalf("ICY metadata = %+v, want artist 'DJ Cool' title 'Night Drive'", got)
	}
}
