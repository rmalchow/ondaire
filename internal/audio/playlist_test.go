package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ensemble/internal/contracts"
	"ensemble/internal/stream"
)

func TestIsPlaylist(t *testing.T) {
	cases := []struct {
		ct, uri string
		want    bool
	}{
		{"audio/x-scpls", "http://h/x", true},
		{"application/pls+xml", "http://h/x", true},
		{"audio/x-mpegurl", "http://h/x", true},
		{"audio/mpegurl", "http://h/x", true},
		{"", "http://h/station.pls", true},
		{"", "http://h/station.m3u", true},
		{"", "http://h/station.m3u8", false}, // HLS not followed
		{"audio/mpeg", "http://h/stream.mp3", false},
		{"", "http://h/stream", false},
	}
	for _, c := range cases {
		if got := isPlaylist(c.ct, c.uri); got != c.want {
			t.Errorf("isPlaylist(%q,%q)=%v want %v", c.ct, c.uri, got, c.want)
		}
	}
}

func TestFirstPlaylistEntry(t *testing.T) {
	pls := "[playlist]\nNumberOfEntries=2\nFile1=http://ip1/stream\nFile2=http://ip2/stream\n"
	if got := firstPlaylistEntry(strings.NewReader(pls), "http://h/x.pls"); got != "http://ip1/stream" {
		t.Errorf("pls first = %q", got)
	}
	m3u := "#EXTM3U\n#EXTINF:-1,Station\nhttp://ip3/live\n"
	if got := firstPlaylistEntry(strings.NewReader(m3u), "http://h/x.m3u"); got != "http://ip3/live" {
		t.Errorf("m3u first = %q", got)
	}
	// Relative entry resolves against the base URL.
	rel := "#EXTM3U\n/live/stream\n"
	if got := firstPlaylistEntry(strings.NewReader(rel), "http://h:8000/x.m3u"); got != "http://h:8000/live/stream" {
		t.Errorf("relative resolve = %q", got)
	}
	// No usable entry.
	if got := firstPlaylistEntry(strings.NewReader("#EXTM3U\n#only comments\n"), "http://h/x.m3u"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestOpenHTTPAuthBasic(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960))
	var gotUser, gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(body)
	}))
	defer srv.Close()

	src, err := OpenHTTPAuth(context.Background(), srv.URL+"/stream",
		&contracts.StreamAuth{Scheme: "basic", User: "alice", Pass: "s3cret"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	if !gotOK || gotUser != "alice" || gotPass != "s3cret" {
		t.Fatalf("basic auth not sent: ok=%v user=%q pass=%q", gotOK, gotUser, gotPass)
	}
	if err := src.ReadFrame(make([]byte, stream.FrameBytes)); err != nil {
		t.Fatalf("read: %v", err)
	}
}

func TestOpenHTTPAuthBearer(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960))
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(body)
	}))
	defer srv.Close()

	src, err := OpenHTTPAuth(context.Background(), srv.URL+"/stream",
		&contracts.StreamAuth{Scheme: "bearer", Token: "tok-xyz"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	if gotAuth != "Bearer tok-xyz" {
		t.Fatalf("bearer header = %q, want 'Bearer tok-xyz'", gotAuth)
	}
}

// A .pls playlist is followed to its first entry, carrying auth to BOTH requests.
func TestOpenHTTPAuthFollowsPlaylist(t *testing.T) {
	body := writeWAVs16(48000, 2, genTone(48000, 2, 440, 960))
	var mux *http.ServeMux
	mux = http.NewServeMux()
	authOK := func(r *http.Request) bool {
		u, p, ok := r.BasicAuth()
		return ok && u == "u" && p == "p"
	}
	var streamHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	mux.HandleFunc("/station.pls", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-scpls")
		w.Write([]byte("[playlist]\nFile1=" + srv.URL + "/live\n"))
	})
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		streamHit = true
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(body)
	})

	src, err := OpenHTTPAuth(context.Background(), srv.URL+"/station.pls",
		&contracts.StreamAuth{Scheme: "basic", User: "u", Pass: "p"})
	if err != nil {
		t.Fatalf("open via playlist: %v", err)
	}
	defer src.Close()
	if !streamHit {
		t.Fatal("playlist not followed to the stream entry")
	}
}
