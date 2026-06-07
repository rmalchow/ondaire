package audio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fileSource is the pull-paced source: a file under MEDIA_DIR decoded through a
// framer, returning real io.EOF at the end (D9).
type fileSource struct {
	f      *os.File
	fr     *framer
	closed bool
}

// openFile constructs a file source for a "file:" URI or a bare path, bounding
// resolution to mediaDir (traversal guard, §6).
func openFile(_ context.Context, uri, mediaDir string) (Source, error) {
	rel := uri
	if i := strings.Index(rel, ":"); i >= 0 && strings.EqualFold(rel[:i], "file") {
		rel = rel[i+1:]
	}
	rel = strings.TrimPrefix(rel, "//") // tolerate file://path

	// Absolute paths escape MEDIA_DIR by definition.
	if filepath.IsAbs(rel) {
		return nil, ErrTraversal
	}
	clean := filepath.Clean(rel)
	full := filepath.Join(mediaDir, clean)

	// Verify the cleaned result stays inside mediaDir.
	relCheck, err := filepath.Rel(mediaDir, full)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return nil, ErrTraversal
	}

	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %v", ErrBadMedia, rel, err)
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(full)), ".")
	dec, err := newDecoder(f, ext)
	if err != nil {
		f.Close()
		return nil, err
	}

	return &fileSource{f: f, fr: newFramer(dec)}, nil
}

func (s *fileSource) ReadFrame(dst []byte) error { return s.fr.frame(dst) }

func (s *fileSource) Live() bool { return false }

func (s *fileSource) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.f.Close()
}
