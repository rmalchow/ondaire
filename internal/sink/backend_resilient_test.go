package sink

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/stream"
)

// fakeBackend is a controllable contracts.Backend for the failover tests: it
// fails Write while `down` is set, and counts opens/writes/closes.
type fakeBackend struct {
	name   string
	down   *atomic.Bool
	opened *int32
	writes *int32
	closed atomic.Bool
}

func (f *fakeBackend) Write(_ []byte) error {
	if f.down.Load() {
		return fmt.Errorf("%s down", f.name)
	}
	atomic.AddInt32(f.writes, 1)
	return nil
}
func (f *fakeBackend) Close() error { f.closed.Store(true); return nil }

// newFakeCand returns a candidate that opens a fakeBackend wired to `down`.
func newFakeCand(name string, down *atomic.Bool, opened, writes *int32) candidate {
	return candidate{kind: "exec", arg: name, label: name,
		openFn: func(*slog.Logger) (contracts.Backend, error) {
			atomic.AddInt32(opened, 1)
			return &fakeBackend{name: name, down: down, opened: opened, writes: writes}, nil
		}}
}

func rframe() []byte { return make([]byte, stream.FrameBytes) } // the null discard validates frame size

// rClock is a manually-advanced clock for deterministic backoff testing.
type rClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *rClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *rClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// TestResilientRotatesToWorkingOutput: the first candidate is down, so the chain
// must rotate to the second (healthy) one and stay there.
func TestResilientRotatesToWorkingOutput(t *testing.T) {
	down1, down2 := &atomic.Bool{}, &atomic.Bool{}
	down1.Store(true) // first output broken
	var o1, w1, o2, w2 int32
	r := newResilientBackend([]candidate{
		newFakeCand("a", down1, &o1, &w1),
		newFakeCand("b", down2, &o2, &w2),
	}, slog.Default())

	for i := 0; i < 5; i++ {
		_ = r.Write(rframe())
	}
	if atomic.LoadInt32(&w2) == 0 {
		t.Fatalf("expected writes to land on healthy candidate b, got w1=%d w2=%d", w1, w2)
	}
}

// TestResilientBacksOffWhenAllDown: with every candidate down, the chain must
// stop hammering and enter the rested state after maxSweeps full passes.
func TestResilientBacksOffWhenAllDown(t *testing.T) {
	down := &atomic.Bool{}
	down.Store(true)
	var o1, w1, o2, w2 int32
	cands := []candidate{
		newFakeCand("a", down, &o1, &w1),
		newFakeCand("b", down, &o2, &w2),
	}
	clk := &rClock{t: time.Unix(1_000_000, 0)}
	r := newResilientBackend(cands, slog.Default())
	r.now = clk.now

	// Drive enough frames to exhaust the sweeps and enter rest.
	for i := 0; i < 50; i++ {
		_ = r.Write(rframe())
	}
	r.mu.Lock()
	resting := !r.restAt.IsZero() && clk.now().Before(r.restAt)
	opensWhileResting := atomic.LoadInt32(&o1) + atomic.LoadInt32(&o2)
	r.mu.Unlock()
	if !resting {
		t.Fatalf("expected the chain to be resting after repeated failures")
	}

	// While resting, further writes must NOT keep opening candidates.
	before := opensWhileResting
	for i := 0; i < 20; i++ {
		_ = r.Write(rframe())
	}
	if got := atomic.LoadInt32(&o1) + atomic.LoadInt32(&o2); got != before {
		t.Fatalf("resting chain kept opening outputs: before=%d after=%d", before, got)
	}

	// After the backoff elapses and a candidate recovers, the chain heals.
	down.Store(false)
	clk.advance(resilientMaxBackoff)
	for i := 0; i < 5; i++ {
		_ = r.Write(rframe())
	}
	if atomic.LoadInt32(&w1)+atomic.LoadInt32(&w2) == 0 {
		t.Fatalf("expected recovery after backoff + candidate up")
	}
}

// TestResilientReviveForcesRetry: a rested chain must retry immediately on Revive
// (the test-tone hook), without waiting for the backoff to elapse.
func TestResilientReviveForcesRetry(t *testing.T) {
	down := &atomic.Bool{}
	down.Store(true)
	var o1, w1 int32
	clk := &rClock{t: time.Unix(1_000_000, 0)}
	r := newResilientBackend([]candidate{newFakeCand("a", down, &o1, &w1)}, slog.Default())
	r.now = clk.now
	for i := 0; i < 20; i++ {
		_ = r.Write(rframe())
	}
	r.mu.Lock()
	resting := !r.restAt.IsZero()
	r.mu.Unlock()
	if !resting {
		t.Fatalf("precondition: chain should be resting")
	}
	down.Store(false)
	r.Revive() // operator poked it (test tone) — retry now, not after backoff
	opensBefore := atomic.LoadInt32(&o1)
	_ = r.Write(rframe())
	if atomic.LoadInt32(&o1) <= opensBefore {
		t.Fatalf("Revive did not force a retry of the chain")
	}
	if atomic.LoadInt32(&w1) == 0 {
		t.Fatalf("expected a write to land after Revive + candidate up")
	}
}

// TestResilientSetPreferredReorders: the override moves a device to the front.
func TestResilientSetPreferredReorders(t *testing.T) {
	r := newResilientBackend([]candidate{alsaCandidate("default"), alsaCandidate("hw:1,0")}, slog.Default())
	r.SetPreferred("hw:1,0")
	if r.cands[0].arg != "hw:1,0" {
		t.Fatalf("SetPreferred did not move hw:1,0 to front: %q", r.cands[0].arg)
	}
	if len(r.cands) != 2 {
		t.Fatalf("SetPreferred changed candidate count: %d", len(r.cands))
	}
}

// TestResilientEmptyChainDiscards: no real outputs => behave like null (no panic).
func TestResilientEmptyChainDiscards(t *testing.T) {
	r := newResilientBackend(nil, slog.Default())
	for i := 0; i < 5; i++ {
		if err := r.Write(rframe()); err != nil {
			t.Fatalf("empty chain Write returned error: %v", err)
		}
	}
}
