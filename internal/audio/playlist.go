package audio

import (
	"bufio"
	"io"
	"net/url"
	"path"
	"strings"
)

// maxPlaylistBytes caps how much of a playlist body we read — these are tiny
// text files; anything larger is not a playlist we follow.
const maxPlaylistBytes = 1 << 20 // 1 MiB

// isPlaylist reports whether a response looks like a PLS or (extended) M3U
// playlist by Content-Type or URL extension. HLS (.m3u8) is deliberately
// excluded — it is a segment playlist, not a single stream URL.
func isPlaylist(contentType, uri string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "audio/x-scpls", "application/pls+xml",
		"audio/x-mpegurl", "audio/mpegurl", "application/x-mpegurl", "application/mpegurl":
		return true
	}
	switch strings.ToLower(strings.TrimPrefix(path.Ext(stripQuery(uri)), ".")) {
	case "pls", "m3u":
		return true
	}
	return false
}

// firstPlaylistEntry returns the first http(s) stream URL in a PLS or M3U body,
// resolved against base for relative entries. Returns "" if none is found.
func firstPlaylistEntry(body io.Reader, base string) string {
	sc := bufio.NewScanner(io.LimitReader(body, maxPlaylistBytes))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue // blank, M3U directive/comment, or PLS section header
		}
		if eq := strings.IndexByte(line, '='); eq >= 0 {
			// A "key=value" line: only PLS "FileN=<url>" entries are URLs; skip
			// metadata keys (NumberOfEntries, TitleN, LengthN, Version, ...).
			if !strings.HasPrefix(strings.ToLower(line), "file") {
				continue
			}
			line = strings.TrimSpace(line[eq+1:])
		}
		if u := resolveEntry(base, line); u != "" {
			return u
		}
	}
	return ""
}

// resolveEntry returns an absolute http(s) URL for a playlist entry, resolving
// relative paths against base. Non-http(s) results are rejected.
func resolveEntry(base, entry string) string {
	lc := strings.ToLower(entry)
	if strings.HasPrefix(lc, "http://") || strings.HasPrefix(lc, "https://") {
		return entry
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	r, err := url.Parse(entry)
	if err != nil {
		return ""
	}
	res := b.ResolveReference(r)
	if res.Scheme == "http" || res.Scheme == "https" {
		return res.String()
	}
	return ""
}

func stripQuery(uri string) string {
	if i := strings.IndexAny(uri, "?#"); i >= 0 {
		return uri[:i]
	}
	return uri
}
