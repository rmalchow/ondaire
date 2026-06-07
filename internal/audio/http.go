package audio

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
)

// httpClient streams infinite bodies: no overall timeout, but a bounded
// dial/response-header timeout so a dead server fails Open promptly.
var httpClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		ResponseHeaderTimeout: 10_000_000_000, // 10s
	},
}

// httpSource is a live-paced source over an HTTP(S) response body.
type httpSource struct {
	*liveReader
	resp *http.Response
}

// openHTTP fetches uri and frames its body live-paced (never EOF; §6.1).
func openHTTP(ctx context.Context, uri, _ string) (Source, error) {
	cctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, uri, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w: http request: %v", ErrBadMedia, err)
	}
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

	format := formatFromContentType(resp.Header.Get("Content-Type"))
	if format == "" {
		format = formatFromExt(uri)
	}

	dec, err := newDecoder(resp.Body, format)
	if err != nil {
		resp.Body.Close()
		cancel()
		return nil, err
	}

	fr := newFramer(dec)
	lr := newLiveReader(fr, cancel, func() { resp.Body.Close() })
	return &httpSource{liveReader: lr, resp: resp}, nil
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
