package audio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"ondaire/internal/contracts"
)

// maxPlaylistDepth bounds .pls/.m3u indirection so a self-referential playlist
// can't loop forever.
const maxPlaylistDepth = 2

// httpClient streams infinite bodies: no overall timeout, but a bounded
// dial/response-header timeout so a dead server fails Open promptly.
var httpClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		ResponseHeaderTimeout: 10_000_000_000, // 10s
	},
}

// httpSource is a live-paced source over an HTTP(S) response body. meta carries
// the latest ICY StreamTitle (now-playing) when the stream supplies it.
type httpSource struct {
	*liveReader
	resp *http.Response
	meta *icyMeta
}

// Metadata implements the optional now-playing channel (D57) from ICY metadata.
// Returns the current track (parsing "Artist - Title" when present); ok=false
// until the stream sends a StreamTitle (the UI then shows the preset name).
func (s *httpSource) Metadata() (contracts.TrackMetadata, bool) {
	if s.meta == nil {
		return contracts.TrackMetadata{}, false
	}
	t := s.meta.get()
	if t == "" {
		return contracts.TrackMetadata{}, false
	}
	if artist, title, ok := splitArtistTitle(t); ok {
		return contracts.TrackMetadata{Artist: artist, Title: title}, true
	}
	return contracts.TrackMetadata{Title: t}, true
}

// openHTTP fetches uri and frames its body live-paced (never EOF; §6.1). It is
// the registry entry (no auth); authenticated presets call OpenHTTPAuth.
func openHTTP(ctx context.Context, uri, _ string) (Source, error) {
	return openHTTPWith(ctx, uri, nil, 0)
}

// OpenHTTPAuth opens an HTTP(S) stream with optional credentials, resolving any
// .pls/.m3u playlist to its first stream entry. Used by the stream:<id> preset
// path, where auth is resolved from cluster state at play time.
func OpenHTTPAuth(ctx context.Context, uri string, auth *contracts.StreamAuth) (Source, error) {
	return openHTTPWith(ctx, uri, auth, 0)
}

func openHTTPWith(ctx context.Context, uri string, auth *contracts.StreamAuth, depth int) (Source, error) {
	cctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, uri, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w: http request: %v", ErrBadMedia, err)
	}
	applyAuth(req, auth)
	req.Header.Set("Icy-MetaData", "1") // opt into ICY in-band now-playing metadata
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w: http get: %v", ErrBadMedia, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("%w: http status %d", ErrBadMedia, resp.StatusCode)
	}

	// Internet-radio stations often hand out a .pls/.m3u pointing at the real
	// stream; follow the first entry (carrying auth) before decoding. HLS (.m3u8)
	// is a segment playlist and is not supported here.
	if isPlaylist(resp.Header.Get("Content-Type"), uri) {
		next := firstPlaylistEntry(resp.Body, uri)
		resp.Body.Close()
		cancel()
		if next == "" {
			return nil, fmt.Errorf("%w: empty playlist", ErrBadMedia)
		}
		if depth >= maxPlaylistDepth {
			return nil, fmt.Errorf("%w: playlist nested too deep", ErrBadMedia)
		}
		return openHTTPWith(ctx, next, auth, depth+1)
	}

	format := formatFromContentType(resp.Header.Get("Content-Type"))
	if format == "" {
		format = formatFromExt(uri)
	}

	// De-interleave ICY metadata when the server advertises an interval, so the
	// decoder sees clean audio and we can surface the now-playing StreamTitle.
	meta := &icyMeta{}
	var body io.Reader = resp.Body
	if mi, _ := strconv.Atoi(resp.Header.Get("icy-metaint")); mi > 0 {
		body = newICYReader(resp.Body, mi, meta)
	}

	dec, err := newDecoder(body, format)
	if err != nil {
		resp.Body.Close()
		cancel()
		return nil, err
	}

	fr := newFramer(dec)
	lr := newLiveReader(fr, cancel, func() { resp.Body.Close() })
	return &httpSource{liveReader: lr, resp: resp, meta: meta}, nil
}

// applyAuth adds optional credentials to the request. A nil/empty-scheme auth is
// a no-op (plain GET, unchanged behavior).
func applyAuth(req *http.Request, auth *contracts.StreamAuth) {
	if auth == nil {
		return
	}
	switch auth.Scheme {
	case "basic":
		req.SetBasicAuth(auth.User, auth.Pass)
	case "bearer":
		if auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+auth.Token)
		}
	}
}

// formatFromContentType maps a Content-Type to a decoder format, or "".
func formatFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/flac", "audio/x-flac":
		return "flac"
	case "audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave":
		return "wav"
	}
	return ""
}

// formatFromExt maps a URL path extension to a decoder format, or "".
func formatFromExt(uri string) string {
	u := uri
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	switch strings.ToLower(strings.TrimPrefix(path.Ext(u), ".")) {
	case "mp3":
		return "mp3"
	case "flac":
		return "flac"
	case "wav", "wave":
		return "wav"
	}
	return ""
}
