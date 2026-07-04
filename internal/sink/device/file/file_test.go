package file

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ondaire/internal/sink/device"
	"ondaire/internal/stream"
)

// compile-time capability assertions for the file adapter.
var (
	_ device.Sink          = (*backend)(nil)
	_ device.Interrupter   = (*backend)(nil)
	_ device.StatsReporter = (*backend)(nil)
)

func frame(fill byte) []byte {
	b := make([]byte, stream.FrameBytes)
	for i := range b {
		b[i] = fill
	}
	return b
}

// fakeClock drives the file pacer deterministically. newTimer returns a timer
// that has ALREADY fired (so the pacing select never waits on the wall clock) and
// advances the virtual clock by exactly the requested wait — modelling a punctual
// scheduler. The set of waits is recorded for cadence assertions.
type fakeClock struct {
	mu    sync.Mutex
	t     time.Time
	waits []time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// newTimer records the wait, advances the virtual clock by it, and returns a
// timer whose channel is already readable so pacing() proceeds without a real wait.
func (c *fakeClock) newTimer(d time.Duration) *time.Timer {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	if d > 0 {
		c.t = c.t.Add(d)
	}
	c.mu.Unlock()
	t := time.NewTimer(0) // fires (almost) immediately; the select below picks t.C
	return t
}

func (c *fakeClock) recorded() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.waits))
	copy(out, c.waits)
	return out
}

// newPaced wires a freshly-opened file backend to the deterministic clock.
func newPaced(t *testing.T, c *fakeClock) (*backend, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.pcm")
	b, err := newBackend(path)
	if err != nil {
		t.Fatalf("newBackend: %v", err)
	}
	b.now = c.now
	b.newTimer = c.newTimer
	return b, path
}

func TestFileAppendsRawPCM(t *testing.T) {
	c := newFakeClock()
	b, path := newPaced(t, c)

	f1, f2 := frame(0xAA), frame(0x55)
	if err := b.Write(f1); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(f2); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte{}, f1...), f2...)
	if !bytes.Equal(got, want) {
		t.Fatalf("file bytes mismatch: got %d bytes, want %d (raw concatenated PCM)", len(got), len(want))
	}
}

// TestFileAppendsToExisting verifies O_APPEND semantics: re-opening the same path
// adds to, rather than truncating, what is already there.
func TestFileAppendsToExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pcm")
	seed := []byte("PRE")
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := newBackend(path)
	if err != nil {
		t.Fatal(err)
	}
	b.pace = false // dump-as-fast-as-possible: no clock needed for this check
	if err := b.Write(frame(0x01)); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, seed) || len(got) != len(seed)+stream.FrameBytes {
		t.Fatalf("append broke: len=%d, want %d (seed preserved + one frame)", len(got), len(seed)+stream.FrameBytes)
	}
}

func TestFileRejectsWrongSize(t *testing.T) {
	c := newFakeClock()
	b, path := newPaced(t, c)
	defer b.Close()

	if err := b.Write(make([]byte, stream.FrameBytes-1)); err == nil {
		t.Fatal("short frame should error")
	}
	if err := b.Write(make([]byte, stream.FrameBytes+1)); err == nil {
		t.Fatal("long frame should error")
	}
	// Nothing should have been written to disk.
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Fatalf("rejected frames touched the file: size=%v err=%v", fiSize(fi), err)
	}
}

func fiSize(fi os.FileInfo) int64 {
	if fi == nil {
		return -1
	}
	return fi.Size()
}

func TestFileBadPath(t *testing.T) {
	if _, err := newBackend("/no/such/dir/file.pcm"); err == nil {
		t.Fatal("bad path should error")
	}
	if _, err := newBackend(""); err == nil {
		t.Fatal("empty path should error")
	}
}

// TestFilePacingDriftFree drives many frames through the injected clock and
// asserts the absolute-deadline accumulator: the first paced frame anchors the
// origin (admitted immediately, no timer), and every subsequent frame waits
// exactly one frame period.
func TestFilePacingDriftFree(t *testing.T) {
	c := newFakeClock()
	b, _ := newPaced(t, c)
	defer b.Close()

	const N = 100
	for i := 0; i < N; i++ {
		if err := b.Write(frame(byte(i))); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	waits := c.recorded()
	// The origin frame is admitted via the IsZero() fast path (no newTimer call),
	// so we expect one timer-backed wait per subsequent frame.
	if len(waits) != N-1 {
		t.Fatalf("recorded %d timed waits, want %d (origin frame is immediate)", len(waits), N-1)
	}
	for i, w := range waits {
		if w != framePeriod {
			t.Fatalf("wait[%d]=%v, want exactly %v (drift!)", i, w, framePeriod)
		}
	}
}

// TestFilePacingAbsorbsLateWakeups proves the accumulator is absolute: a clock
// slip before a frame shrinks the next wait to the remainder rather than a fresh
// full period.
func TestFilePacingAbsorbsLateWakeups(t *testing.T) {
	c := newFakeClock()
	b, _ := newPaced(t, c)
	defer b.Close()

	if err := b.Write(frame(1)); err != nil { // origin frame, immediate (frames now 1)
		t.Fatal(err)
	}
	// Slip 5 ms after the origin but before frame 2.
	c.mu.Lock()
	c.t = c.t.Add(5 * time.Millisecond)
	c.mu.Unlock()
	if err := b.Write(frame(2)); err != nil { // deadline = origin + 1·period
		t.Fatal(err)
	}
	waits := c.recorded()
	if len(waits) != 1 {
		t.Fatalf("recorded %d waits, want 1", len(waits))
	}
	if want := framePeriod - 5*time.Millisecond; waits[0] != want {
		t.Fatalf("late-wakeup wait=%v, want %v (accumulator must absorb the slip)", waits[0], want)
	}
}

// TestFileInterruptUnblocks parks a paced Write on a slow (real) timer, then
// proves Interrupt releases it promptly (without error) and that subsequent
// Writes fail fast as "closed".
func TestFileInterruptUnblocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pcm")
	b, err := newBackend(path)
	if err != nil {
		t.Fatal(err)
	}
	// Anchor the origin with one immediate frame.
	if err := b.Write(frame(1)); err != nil {
		t.Fatal(err)
	}
	// Pin the clock at the cadence origin (b.start) so the next frame's deadline
	// (start + 1·framePeriod) is always in the future — wait > 0 regardless of how
	// slow this (CI) box was during the first append. Capturing b.now() here would
	// race the real clock instead: if that append took ≥ one framePeriod, wait ≤ 0
	// and pacing() admits immediately without ever building a timer (the flake). Only
	// Interrupt (closing done) can then release the select.
	b.mu.Lock()
	origin := b.start
	b.mu.Unlock()
	b.now = func() time.Time { return origin }
	// A long real timer that we rely on Interrupt to beat; signal when the pacer
	// reaches it so we interrupt deterministically (no real sleep handshake).
	armed := make(chan struct{}, 1)
	b.newTimer = func(time.Duration) *time.Timer {
		armed <- struct{}{}
		return time.NewTimer(time.Hour)
	}

	done := make(chan error, 1)
	go func() { done <- b.Write(frame(2)) }()

	select {
	case <-armed: // the pacer has built its timer and is about to/has entered the select
	case <-time.After(time.Second):
		t.Fatal("paced Write never reached the timer wait")
	}
	b.Interrupt()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("interrupted Write should return a 'closed' error after Interrupt")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt did not unblock the in-flight paced Write")
	}

	// Subsequent Writes must return promptly with the closed error.
	got := make(chan error, 1)
	go func() { got <- b.Write(frame(3)) }()
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("post-interrupt Write should fail fast as closed")
		}
	case <-time.After(time.Second):
		t.Fatal("post-interrupt Write blocked; should fail fast")
	}
	_ = b.Close()
}

func TestFileCloseClosesAndIdempotent(t *testing.T) {
	c := newFakeClock()
	b, path := newPaced(t, c)
	if err := b.Write(frame(7)); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	if err := b.Close(); err != nil {
		t.Fatalf("second Close returned %v, want nil", err)
	}
	// Writing after Close fails (file handle gone). Pacing is off-path here because
	// the origin is already set; the closed `done` makes the wait fast.
	if err := b.Write(frame(8)); err == nil {
		t.Fatal("Write after Close should error")
	}
	// The single pre-close frame is the only thing on disk.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(stream.FrameBytes) {
		t.Fatalf("size=%d, want %d", fi.Size(), stream.FrameBytes)
	}
}

func TestFileDeviceStatsKind(t *testing.T) {
	c := newFakeClock()
	b, _ := newPaced(t, c)
	defer b.Close()
	if err := b.Write(frame(1)); err != nil {
		t.Fatal(err)
	}
	st := b.DeviceStats()
	if st.Kind != "file" {
		t.Fatalf("Kind=%q, want %q", st.Kind, "file")
	}
	if st.QueueValid {
		t.Fatal("file must report QueueValid=false")
	}
	if st.FramesWritten != 1 {
		t.Fatalf("FramesWritten=%d, want 1", st.FramesWritten)
	}
}
