// Package file is the file device adapter: it appends raw canonical PCM frames to
// a file on disk (a debug/capture sink). Unlike alsa/exec it has no hardware queue
// to push back on, so — per the device package CONTRACT — it must SYNTHESISE the
// playout rate from an internal real-time clock: Write blocks one frame-period per
// call so the engine is paced exactly as if a real device were draining it.
//
// The pacer is a monotonic deadline accumulator (deadline = start + n·framePeriod),
// not a per-call sleep(framePeriod): the latter re-bases on every wakeup and lets
// scheduling/IO jitter accumulate into unbounded drift. By sleeping only the
// remainder to the next absolute deadline, transient lateness is absorbed and the
// long-run cadence stays locked to nominal — the engine then holds the resample
// ratio at ~1 with no phase servo (file exposes no DelayReporter).
package file

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"ensemble/internal/sink/device"
	"ensemble/internal/stream"
)

// framePeriod is the nominal wall-clock duration of one canonical frame (20 ms).
const framePeriod = time.Duration(stream.FrameNanos) * time.Nanosecond

// backend appends raw PCM to a file and paces itself to one frame per framePeriod
// off an internal real-time clock. It is the file device adapter.
type backend struct {
	// Clock seams — injectable so tests run without real sleeps. Defaults wire to
	// time.Now / time.NewTimer in newBackend.
	now      func() time.Time
	newTimer func(time.Duration) *time.Timer
	pace     bool // false ⇒ "dump as fast as possible" test mode (no clock wait)

	// done is closed once by Interrupt/Close; an in-flight pacing sleep selects on
	// it to abort, and subsequent Writes observe it closed and return promptly.
	done     chan struct{}
	doneOnce sync.Once

	mu       sync.Mutex // guards the fields below
	f        *os.File
	start    time.Time // monotonic origin of the cadence (set on first paced Write)
	frames   uint64    // FramesWritten (frames appended to the file)
	writeErr uint64    // WriteErrors (append failures)
}

// newBackend opens path for append and returns a real-time-paced file sink. An
// empty path is an error (as the legacy backend required). The clock seams default
// to the real clock with pacing enabled.
func newBackend(path string) (*backend, error) {
	if path == "" {
		return nil, fmt.Errorf("file backend: empty path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("file backend: %w", err)
	}
	return &backend{
		now:      time.Now,
		newTimer: time.NewTimer,
		pace:     true,
		done:     make(chan struct{}),
		f:        f,
	}, nil
}

// Write paces to the next cadence deadline (blocking, per the contract), then
// appends the raw frame. It returns an error only on a wrong frame size, a closed
// sink, or a permanent file error — never to signal mere backpressure.
func (b *backend) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("file: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}

	// Pace BEFORE the append so the very first frame still costs one period — the
	// engine's clock is the time spent here, independent of how fast the disk is.
	if err := b.pacing(); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		return fmt.Errorf("file: closed")
	}
	if _, err := b.f.Write(frame); err != nil {
		b.writeErr++
		return err
	}
	b.frames++
	return nil
}

// pacing sleeps until the next monotonic cadence deadline, returning early (without
// error) if the sink is interrupted/closed. The deadline is an absolute accumulator
// anchored at the first call, so per-wakeup jitter never accumulates into drift.
func (b *backend) pacing() error {
	if !b.pace {
		// Still honour an already-fired interrupt so a closed sink fails fast.
		select {
		case <-b.done:
			return fmt.Errorf("file: closed")
		default:
			return nil
		}
	}

	b.mu.Lock()
	if b.start.IsZero() {
		// First paced frame defines the cadence origin and is admitted immediately;
		// subsequent frames are clamped to start + n·framePeriod.
		b.start = b.now()
		b.mu.Unlock()
		select {
		case <-b.done:
			return fmt.Errorf("file: closed")
		default:
			return nil
		}
	}
	// Next deadline = start + frames·framePeriod (frames already played sits at the
	// count of admitted frames, so the n-th frame targets origin + n periods).
	deadline := b.start.Add(time.Duration(b.frames) * framePeriod)
	b.mu.Unlock()

	wait := deadline.Sub(b.now())
	if wait <= 0 {
		// We are at or behind the deadline (disk stall, scheduler hiccup): admit now
		// and let the absolute accumulator pull the cadence back — never sleep extra.
		select {
		case <-b.done:
			return fmt.Errorf("file: closed")
		default:
			return nil
		}
	}

	t := b.newTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-b.done:
		return fmt.Errorf("file: closed")
	}
}

// Interrupt aborts an in-flight pacing sleep and marks the sink closed so further
// Writes return promptly. It takes no lock the Write path holds during its sleep
// (Write releases b.mu before selecting), so it can never deadlock against a
// blocked Write. Idempotent.
func (b *backend) Interrupt() { b.doneOnce.Do(func() { close(b.done) }) }

// Close stops pacing and closes the file. Idempotent.
func (b *backend) Close() error {
	b.Interrupt() // unblock any sleeper, mark closed
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		return nil
	}
	err := b.f.Close()
	b.f = nil
	return err
}

// DeviceStats reports file telemetry. The queue is meaningless for a file sink —
// there is no buffer between Write and a speaker — so QueueValid is false and
// QueueNs is 0. Pacing is exact (the internal clock), so there are no underruns.
func (b *backend) DeviceStats() device.DeviceStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return device.DeviceStats{
		Kind:          "file",
		QueueNs:       0,
		QueueValid:    false,
		FramesWritten: b.frames,
		WriteErrors:   b.writeErr,
	}
}

func init() {
	// nil `available` ⇒ always usable; file is explicit-only, so no enumerator and no
	// failover candidates (it bypasses the failover chain).
	device.Register("file", func(arg string, _ *slog.Logger) (device.Sink, error) {
		return newBackend(arg)
	}, nil)
}
