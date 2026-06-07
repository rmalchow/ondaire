package sink

import (
	"fmt"
	"sync"
	"time"

	"ensemble/internal/stream"
)

// nullBackend discards frames; optionally paces one frame per FrameDuration so
// playout timing is exercised like a real device. Tests disable pacing.
type nullBackend struct {
	mu      sync.Mutex
	written uint64
	last    time.Time
	pace    bool
	sleep   func(time.Duration) // injectable; default time.Sleep
}

func newNullBackend() *nullBackend {
	return &nullBackend{sleep: time.Sleep}
}

func (b *nullBackend) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("null: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pace {
		now := time.Now()
		if !b.last.IsZero() {
			elapsed := now.Sub(b.last)
			period := time.Duration(stream.FrameDuration) * time.Millisecond
			if d := period - elapsed; d > 0 {
				b.sleep(d)
			}
		}
		b.last = time.Now()
	}
	b.written++
	return nil
}

func (b *nullBackend) Close() error { return nil }

func (b *nullBackend) Written() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written
}
