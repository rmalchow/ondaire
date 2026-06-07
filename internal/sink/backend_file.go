package sink

import (
	"fmt"
	"os"
	"sync"

	"ensemble/internal/stream"
)

// fileBackend appends raw PCM to a debug file (no pacing; the scheduler paces).
type fileBackend struct {
	mu sync.Mutex
	f  *os.File
}

func newFileBackend(path string) (*fileBackend, error) {
	if path == "" {
		return nil, fmt.Errorf("file backend: empty path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("file backend: %w", err)
	}
	return &fileBackend{f: f}, nil
}

func (b *fileBackend) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("file: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		return fmt.Errorf("file: closed")
	}
	_, err := b.f.Write(frame)
	return err
}

func (b *fileBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		return nil
	}
	err := b.f.Close()
	b.f = nil
	return err
}
