package sink

import (
	"sync"
	"testing"
	"time"
)

// closeRecBackend records writes and tracks whether Close was called.
type closeRecBackend struct {
	mu     sync.Mutex
	frames int
	closed bool
}

func (b *closeRecBackend) Write(f []byte) error {
	b.mu.Lock()
	b.frames++
	b.mu.Unlock()
	return nil
}

func (b *closeRecBackend) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

func (b *closeRecBackend) wasClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func TestSwapBackendClosesOldAndRoutesWrites(t *testing.T) {
	clk := newFakeClock(true)
	old := &closeRecBackend{}
	p := newTestPlayout(t, clk, old, nil)
	defer p.Close()

	nb := &closeRecBackend{}
	p.SwapBackend(nb)

	if !old.wasClosed() {
		t.Fatal("old backend not closed after swap")
	}
	p.mu.Lock()
	got := p.out
	p.mu.Unlock()
	if got != nb {
		t.Fatal("playout did not adopt the new backend")
	}
}

func TestSwapBackendNilIsNoop(t *testing.T) {
	clk := newFakeClock(true)
	be := &closeRecBackend{}
	p := newTestPlayout(t, clk, be, nil)
	defer p.Close()
	p.SwapBackend(nil)
	if be.wasClosed() {
		t.Fatal("nil swap closed the live backend")
	}
}

func TestSwapBackendAfterCloseClosesNew(t *testing.T) {
	clk := newFakeClock(true)
	be := &closeRecBackend{}
	p := newTestPlayout(t, clk, be, nil)
	_ = p.Close()
	time.Sleep(5 * time.Millisecond)

	nb := &closeRecBackend{}
	p.SwapBackend(nb)
	if !nb.wasClosed() {
		t.Fatal("swap after Close should close the rejected backend")
	}
}
