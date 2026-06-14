package null

import (
	"sync"
	"testing"
	"time"

	"ensemble/internal/sink/device"
	"ensemble/internal/stream"
)

// compile-time capability assertions: null honours Sink plus the optional
// Interrupter and StatsReporter, and deliberately NOTHING else (no Delay/Latency
// — null has no measurable queue; see the package doc).
var (
	_ device.Sink          = (*Null)(nil)
	_ device.Interrupter   = (*Null)(nil)
	_ device.StatsReporter = (*Null)(nil)
)

func frame() []byte { return make([]byte, stream.FrameBytes) }

// fakeClock is a manually-advanced clock plus a deterministic sleep seam. sleep
// does NOT advance the clock by itself — the test advances time explicitly and
// records the durations it was asked to wait, so pacing is verified without any
// real wall-clock blocking.
type fakeClock struct {
	mu     sync.Mutex
	t      time.Time
	waits  []time.Duration // every duration handed to sleep, in order
	done   <-chan struct{} // when set, sleep returns early if it is closed
	woke   int             // number of sleeps that returned via done (interrupted)
	napped int             // number of sleeps that "completed" their wait
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Unix(1_700_000_000, 0)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// sleep records the requested wait and advances the virtual clock by exactly
// that amount (a perfectly punctual scheduler). If the done channel is already
// closed it returns immediately without advancing — modelling Interrupt aborting
// an in-flight wait.
func (c *fakeClock) sleep(d time.Duration) {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	done := c.done
	c.mu.Unlock()

	if done != nil {
		select {
		case <-done:
			c.mu.Lock()
			c.woke++
			c.mu.Unlock()
			return
		default:
		}
	}
	c.mu.Lock()
	c.napped++
	if d > 0 {
		c.t = c.t.Add(d)
	}
	c.mu.Unlock()
}

func (c *fakeClock) recorded() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.waits))
	copy(out, c.waits)
	return out
}

// newPaced builds a Null wired to a deterministic clock/sleep so the test drives
// pacing without real 20 ms waits. It mirrors New() but swaps the seams.
func newPaced(c *fakeClock) *Null {
	n := New()
	n.now = c.now
	n.sleep = c.sleep
	c.mu.Lock()
	c.done = n.done // let the fake sleep observe the same Interrupt signal
	c.mu.Unlock()
	return n
}

func TestNullRejectsWrongSize(t *testing.T) {
	n := New()
	defer n.Close()
	if err := n.Write(make([]byte, stream.FrameBytes-1)); err == nil {
		t.Fatal("short frame should error")
	}
	if err := n.Write(make([]byte, stream.FrameBytes+1)); err == nil {
		t.Fatal("long frame should error")
	}
	if err := n.Write(nil); err == nil {
		t.Fatal("nil frame should error")
	}
	if n.Written() != 0 {
		t.Fatalf("rejected frames counted: written=%d, want 0", n.Written())
	}
}

func TestNullWrittenCounts(t *testing.T) {
	c := newFakeClock()
	n := newPaced(c)
	defer n.Close()
	const N = 7
	for i := 0; i < N; i++ {
		if err := n.Write(frame()); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if n.Written() != N {
		t.Fatalf("written=%d, want %d", n.Written(), N)
	}
	st := n.DeviceStats()
	if st.FramesWritten != N {
		t.Fatalf("DeviceStats.FramesWritten=%d, want %d", st.FramesWritten, N)
	}
}

// TestNullPacingDriftFree verifies the absolute-deadline accumulator: the first
// frame anchors the origin and is admitted immediately (wait 0), and every later
// frame targets start+n·framePeriod. Because the fake clock advances by exactly
// the wait each time, the accumulator never drifts: after k frames we have waited
// a total of (k-1)·framePeriod and each individual wait is exactly one period.
func TestNullPacingDriftFree(t *testing.T) {
	c := newFakeClock()
	n := newPaced(c)
	defer n.Close()

	const N = 100
	for i := 0; i < N; i++ {
		if err := n.Write(frame()); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	waits := c.recorded()
	if len(waits) != N {
		t.Fatalf("recorded %d waits, want one per Write (%d)", len(waits), N)
	}
	// With a perfectly-punctual scheduler (the fake advances the clock by exactly
	// the wait), every frame targets the next absolute deadline, so every wait is
	// exactly one frame period — no accumulating slop on any frame.
	for i := 0; i < N; i++ {
		if waits[i] != framePeriod {
			t.Fatalf("wait[%d]=%v, want exactly %v (drift!)", i, waits[i], framePeriod)
		}
	}
	// Total virtual time elapsed == N periods: the long-run cadence is locked to
	// the frame rate.
	wantElapsed := time.Duration(N) * framePeriod
	if got := c.now().Sub(time.Unix(1_700_000_000, 0)); got != wantElapsed {
		t.Fatalf("elapsed virtual time=%v, want %v", got, wantElapsed)
	}
}

// TestNullPacingAbsorbsLateWakeups proves the accumulator is absolute, not a flat
// per-call sleep: if a wake-up is LATE (the clock jumps past a deadline between
// the origin and the next Write), the next wait shrinks to just the remainder so
// the mean cadence stays nailed to the frame rate instead of biasing slow.
func TestNullPacingAbsorbsLateWakeups(t *testing.T) {
	c := newFakeClock()
	n := newPaced(c)
	defer n.Close()

	if err := n.Write(frame()); err != nil { // frame 1: anchors origin, wait 0
		t.Fatal(err)
	}
	// Simulate a scheduler hiccup: extra 5 ms elapses before frame 2.
	c.advance(5 * time.Millisecond)
	if err := n.Write(frame()); err != nil { // frame 2: deadline=origin+20ms
		t.Fatal(err)
	}
	waits := c.recorded()
	// Deadline for frame 2 is origin+20ms; now is origin+5ms ⇒ wait the 15 ms
	// remainder, NOT a fresh 20 ms.
	if want := framePeriod - 5*time.Millisecond; waits[1] != want {
		t.Fatalf("late-wakeup wait=%v, want %v (accumulator must absorb the slip)", waits[1], want)
	}
}

// TestNullInterruptUnblocks proves Interrupt aborts an in-flight blocking Write
// and that subsequent Writes return promptly (closed ⇒ no pacing wait). The sleep
// seam is replaced with one that blocks ONLY on the sink's done channel — so the
// wait is unbounded and can be released exclusively by Interrupt, never by a real
// timer. This isolates the Interrupt path with no real wall-clock dependency.
func TestNullInterruptUnblocks(t *testing.T) {
	n := New()
	entered := make(chan struct{}, 1)
	n.sleep = func(time.Duration) {
		// Signal we are parked, then block until Interrupt closes done.
		select {
		case entered <- struct{}{}:
		default:
		}
		<-n.done
	}

	done := make(chan error, 1)
	go func() { done <- n.Write(frame()) }() // first paced frame parks in the seam above

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Write never entered the pacing sleep")
	}
	n.Interrupt()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("interrupted Write returned error: %v (want nil)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt did not unblock the in-flight Write")
	}

	// Subsequent Writes must return promptly (closed ⇒ no blocking).
	for i := 0; i < 5; i++ {
		got := make(chan error, 1)
		go func() { got <- n.Write(frame()) }()
		select {
		case err := <-got:
			if err != nil {
				t.Fatalf("post-interrupt Write returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("post-interrupt Write blocked; should return promptly")
		}
	}
}

func TestNullDeviceStatsKind(t *testing.T) {
	n := New()
	defer n.Close()
	st := n.DeviceStats()
	if st.Kind != "null" {
		t.Fatalf("Kind=%q, want %q", st.Kind, "null")
	}
	if st.QueueValid {
		t.Fatal("null must report QueueValid=false (no measurable queue)")
	}
}

func TestNullCloseIdempotent(t *testing.T) {
	n := New()
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("second Close returned %v, want nil (idempotent)", err)
	}
}
