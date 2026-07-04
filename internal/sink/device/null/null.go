// Package null is the discard output adapter on the device port. It throws every
// frame away but, crucially, still HONOURS the port contract: Write blocks one
// frame-period per call so the engine is paced exactly as a real device would
// pace it. A discard sink with no internal clock would return instantly and spin
// the playout engine at 100% CPU — so this adapter carries its own real-time
// clock and synthesises the backpressure (device.go, the package CONTRACT).
//
// The pacing clock is drift-free: deadlines accrue on an absolute accumulator
// (next = start + n·framePeriod) rather than a flat sleep per call, so jitter in
// any one wake-up does not bias the long-run rate.
//
// Capabilities: Sink, Interrupter, StatsReporter. There is deliberately no
// DelayReporter/LatencyReporter — null has no queue and no measurable latency, so
// the engine treats its phase as opaque (holds the ratio near 1; see device.go).
package null

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ondaire/internal/sink/device"
	"ondaire/internal/stream"
)

// framePeriod is the exact wall-clock duration one canonical frame represents.
const framePeriod = time.Duration(stream.FrameNanos) // 20 ms, in ns

func init() {
	// Always available; no host capability gate. No enumerator and no failover
	// candidates — null is not a real output, it never joins the device list or
	// the failover chain.
	device.Register("null", func(_ string, _ *slog.Logger) (device.Sink, error) {
		return New(), nil
	}, nil)
}

// Null is the discard sink. Write paces one frame-period via an internal,
// drift-free clock and then drops the frame.
type Null struct {
	mu      sync.Mutex
	written uint64    // frames accepted (telemetry + Written())
	start   time.Time // wall-clock origin of the pacing accumulator; zero until first Write
	n       int64     // number of frame-periods scheduled so far (the accumulator)
	closed  bool      // set by Close/Interrupt; Write stops blocking once closed

	// Pacing toggle and injectable clock — the contract DEFAULT is real-time
	// paced (pace=true). Tests flip pace off, or swap now/sleep, to run without
	// real wall-clock waits.
	pace  bool
	now   func() time.Time
	sleep func(time.Duration) // interruptible sleep; honours the done channel
	done  chan struct{}       // closed by Interrupt to abort an in-flight wait
}

// New builds a real-time-paced null sink (the contract default).
func New() *Null {
	n := &Null{
		pace: true,
		now:  time.Now,
		done: make(chan struct{}),
	}
	n.sleep = n.realSleep
	return n
}

// realSleep waits d, but returns early if Interrupt closes done. This is what
// keeps Close/Interrupt snappy even while a Write is mid-pace.
func (n *Null) realSleep(d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-n.done:
	}
}

// Write blocks for one frame-period (the rate pacer), then discards the frame.
//
// The wait is computed against an absolute deadline accumulator so it is
// drift-free: each call advances n and sleeps until start+n·framePeriod, not for
// a flat framePeriod. A slow wake-up on one frame is absorbed by a shorter wait
// on the next, keeping the mean cadence locked to the frame rate. Once closed
// (Interrupt/Close) Write stops blocking and returns nil — never an error for
// mere backpressure.
func (n *Null) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("null: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}

	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	if n.pace {
		now := n.now()
		if n.start.IsZero() {
			// First frame sets the origin and is not delayed — pacing applies
			// from the second frame onward, matching a freshly-primed device.
			n.start = now
		}
		n.n++
		deadline := n.start.Add(time.Duration(n.n) * framePeriod)
		wait := deadline.Sub(now)
		sleep := n.sleep
		// Release the lock across the wait: Interrupt must not block on it, and
		// concurrent Written()/DeviceStats() calls stay responsive.
		n.mu.Unlock()
		sleep(wait)
		n.mu.Lock()
	}
	n.written++
	n.mu.Unlock()
	return nil
}

// Close stops the sink and unblocks any in-flight Write. Idempotent.
func (n *Null) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.markClosedLocked()
	return nil
}

// Interrupt aborts an in-flight blocking Write (device.Interrupter) and marks the
// sink closed. Closing done releases realSleep without touching the mutex the
// Write path holds only outside its wait.
func (n *Null) Interrupt() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.markClosedLocked()
}

// markClosedLocked sets closed and closes done exactly once. Caller holds mu.
func (n *Null) markClosedLocked() {
	if n.closed {
		return
	}
	n.closed = true
	close(n.done)
}

// Written reports the number of frames accepted. Used by tests.
func (n *Null) Written() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.written
}

// DeviceStats reports null's telemetry (device.StatsReporter). No queue, so
// QueueValid is false; no underruns or write errors are possible.
func (n *Null) DeviceStats() device.DeviceStats {
	n.mu.Lock()
	defer n.mu.Unlock()
	return device.DeviceStats{
		Kind:          "null",
		QueueValid:    false,
		FramesWritten: n.written,
	}
}
