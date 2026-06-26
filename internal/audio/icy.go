package audio

import (
	"io"
	"strings"
	"sync"
)

// ICY (SHOUTcast/Icecast) in-band metadata. When the client sends
// "Icy-MetaData: 1" and the server replies with an "icy-metaint" byte interval,
// the body interleaves a metadata block (a length byte ×16, then NUL-padded
// "StreamTitle='…';") every metaint audio bytes. icyReader de-interleaves those
// blocks so the decoder sees clean audio, and tracks the latest StreamTitle as
// the now-playing track (D57 metadata channel).

// icyMeta holds the latest StreamTitle, read concurrently by the session's
// metadata poll and written by the source's read goroutine.
type icyMeta struct {
	mu    sync.Mutex
	title string
}

func (m *icyMeta) set(t string) { m.mu.Lock(); m.title = t; m.mu.Unlock() }
func (m *icyMeta) get() string  { m.mu.Lock(); defer m.mu.Unlock(); return m.title }

// icyReader strips ICY metadata blocks from r, exposing only audio bytes.
type icyReader struct {
	r       io.Reader
	metaint int
	left    int // audio bytes remaining before the next metadata block
	meta    *icyMeta
}

func newICYReader(r io.Reader, metaint int, meta *icyMeta) *icyReader {
	return &icyReader{r: r, metaint: metaint, left: metaint, meta: meta}
}

func (ic *icyReader) Read(p []byte) (int, error) {
	if ic.left == 0 {
		if err := ic.consumeMeta(); err != nil {
			return 0, err
		}
		ic.left = ic.metaint
	}
	n := len(p)
	if n > ic.left {
		n = ic.left // never cross a metadata boundary in one read
	}
	m, err := ic.r.Read(p[:n])
	ic.left -= m
	return m, err
}

func (ic *icyReader) consumeMeta() error {
	var lb [1]byte
	if _, err := io.ReadFull(ic.r, lb[:]); err != nil {
		return err
	}
	blkLen := int(lb[0]) * 16
	if blkLen == 0 {
		return nil // no metadata this interval
	}
	buf := make([]byte, blkLen)
	if _, err := io.ReadFull(ic.r, buf); err != nil {
		return err
	}
	if t := parseStreamTitle(buf); t != "" {
		ic.meta.set(t)
	}
	return nil
}

// parseStreamTitle extracts the StreamTitle value from a NUL-padded ICY block.
func parseStreamTitle(b []byte) string {
	s := string(b)
	const key = "StreamTitle='"
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	s = s[i+len(key):]
	if j := strings.Index(s, "';"); j >= 0 {
		s = s[:j]
	} else if j := strings.IndexByte(s, '\''); j >= 0 {
		s = s[:j]
	}
	return strings.TrimRight(strings.TrimSpace(s), "\x00")
}

// splitArtistTitle parses the common ICY "Artist - Title" StreamTitle form.
func splitArtistTitle(s string) (artist, title string, ok bool) {
	if i := strings.Index(s, " - "); i > 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+3:]), true
	}
	return "", "", false
}
