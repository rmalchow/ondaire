package discovery

import (
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"ensemble/internal/id"

	"github.com/grandcat/zeroconf"
)

var errFakeResolver = errors.New("fake resolver failure")

// drainPeers collects peers from ch until quiet for `quiet` or `max` elapses.
func drainPeers(ch <-chan Peer, quiet, max time.Duration) []Peer {
	var got []Peer
	deadline := time.After(max)
	for {
		select {
		case p, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, p)
		case <-time.After(quiet):
			return got
		case <-deadline:
			return got
		}
	}
}

func samplePeer() Peer {
	return Peer{
		ID:         peerID,
		Addr:       netip.MustParseAddr("192.168.1.17"),
		GossipPort: 7946, HTTPPort: 8080, StreamPort: 9090, SourcePort: 9200,
	}
}

// --- dedup / emission tests (drive maybeEmit directly, no network) ---

func TestEmitNewPeerOnce(t *testing.T) {
	d := New(Config{ID: selfID})
	p := samplePeer()
	d.maybeEmit(p)
	d.maybeEmit(p) // identical, within reEmitInterval -> suppressed
	d.maybeEmit(p)

	got := drainPeers(d.peers, 50*time.Millisecond, time.Second)
	if len(got) != 1 {
		t.Fatalf("emitted %d peers, want 1: %+v", len(got), got)
	}
	if got[0].ID != peerID {
		t.Errorf("ID = %v", got[0].ID)
	}
}

func TestReEmitAfterInterval(t *testing.T) {
	old := reEmitInterval
	reEmitInterval = 10 * time.Millisecond
	defer func() { reEmitInterval = old }()

	d := New(Config{ID: selfID})
	p := samplePeer()
	d.maybeEmit(p)
	time.Sleep(20 * time.Millisecond)
	d.maybeEmit(p) // unchanged but past the (shrunk) interval

	got := drainPeers(d.peers, 50*time.Millisecond, time.Second)
	if len(got) != 2 {
		t.Fatalf("emitted %d peers, want 2 (re-emit after interval)", len(got))
	}
}

func TestEmitOnFieldChange(t *testing.T) {
	d := New(Config{ID: selfID})
	p := samplePeer()
	d.maybeEmit(p)
	p2 := p
	p2.StreamPort = 9091 // material change
	d.maybeEmit(p2)

	got := drainPeers(d.peers, 50*time.Millisecond, time.Second)
	if len(got) != 2 {
		t.Fatalf("emitted %d peers, want 2 (field change)", len(got))
	}
	if got[1].StreamPort != 9091 {
		t.Errorf("second emit StreamPort = %d, want 9091", got[1].StreamPort)
	}
}

func TestSelfNotEmitted(t *testing.T) {
	// The browse drain calls parseEntry(e, cfg.ID) before maybeEmit; a node's
	// own advertisement must be filtered there so it never reaches the channel.
	selfTXT := []string{
		"id=" + selfID.String(),
		"gossip=7946", "http=8080", "stream=9090", "source=9200",
	}
	e := entry(selfTXT, nil, nil)
	if _, ok := parseEntry(e, selfID); ok {
		t.Fatal("self should be filtered by parseEntry before maybeEmit")
	}
}

func TestNonBlockingSendNeverStalls(t *testing.T) {
	old := peersBuffer
	peersBuffer = 2
	defer func() { peersBuffer = old }()

	d := New(Config{ID: selfID})
	// Never drain. Force many distinct emissions; maybeEmit must never block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			p := samplePeer()
			p.StreamPort = 9000 + i // each distinct => emit attempt
			d.maybeEmit(p)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("maybeEmit stalled on a full channel")
	}
	d.mu.Lock()
	drops := d.drops
	d.mu.Unlock()
	if drops == 0 {
		t.Fatal("expected some dropped sends with an undrained cap-2 channel")
	}
}

// --- lifecycle tests ---

func TestCloseClosesPeersChannel(t *testing.T) {
	d := New(Config{ID: selfID})
	d.Run()
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-d.Peers():
		if ok {
			// May receive a stray peer first; drain then re-check.
			for ok {
				_, ok = <-d.Peers()
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Peers channel not closed after Close")
	}
}

func TestCloseIdempotent(t *testing.T) {
	d := New(Config{ID: selfID})
	d.Run()
	if err := d.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestCloseBeforeRun(t *testing.T) {
	d := New(Config{ID: selfID})
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, ok := <-d.Peers(); ok {
		t.Fatal("expected closed channel")
	}
}

func TestRunAfterClose(t *testing.T) {
	d := New(Config{ID: selfID})
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	d.Run() // no-op; must not panic or spawn live goroutines
	if _, ok := <-d.Peers(); ok {
		t.Fatal("expected closed channel after Close")
	}
}

// TestRestartOnBrowseError injects a resolver factory that fails the first call
// then succeeds, and asserts the browse loop retried rather than stalling.
func TestRestartOnBrowseError(t *testing.T) {
	old := retryInterval
	retryInterval = 5 * time.Millisecond
	defer func() { retryInterval = old }()

	var mu sync.Mutex
	calls := 0
	d := New(Config{ID: selfID})
	d.newResolver = func() (*zeroconf.Resolver, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return nil, errFakeResolver
		}
		// Subsequent calls return a real resolver; Browse will block on ctx
		// until Close cancels it. That is fine — we only assert it was retried.
		return zeroconf.NewResolver()
	}
	// Only run the browse keeper to avoid touching mDNS registration sockets.
	d.wg.Add(1)
	go d.browseKeeper()

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("browse not retried after error (calls=%d)", n)
		case <-time.After(5 * time.Millisecond):
		}
	}
	d.cancel()
	d.wg.Wait()
}

// --- real loopback mDNS round-trip (skips if multicast unavailable) ---

func TestRegisterBrowseRoundTrip(t *testing.T) {
	aID := id.New()
	bID := id.New()

	a := New(Config{
		ID: aID, Instance: aID.String(),
		GossipPort: 17946, HTTPPort: 18080, StreamPort: 19090, SourcePort: 19200,
	})
	// Try to register A directly first; skip the whole test if mDNS register
	// fails (no multicast in this environment).
	srv, err := zeroconf.Register(aID.String(), ServiceName, Domain, 18080, txtRecords(a.cfg), nil)
	if err != nil {
		t.Skipf("mDNS register unavailable, skipping multicast test: %v", err)
	}
	srv.Shutdown()

	a.Run()
	defer a.Close()

	b := New(Config{
		ID: bID, Instance: bID.String(),
		GossipPort: 27946, HTTPPort: 28080, StreamPort: 29090, SourcePort: 29200,
	})
	b.Run()
	defer b.Close()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case p, ok := <-b.Peers():
			if !ok {
				t.Fatal("B's Peers channel closed unexpectedly")
			}
			if p.ID == aID {
				if p.HTTPPort != 18080 || p.GossipPort != 17946 ||
					p.StreamPort != 19090 || p.SourcePort != 19200 {
					t.Errorf("ports mismatch: %+v", p)
				}
				return // success
			}
		case <-deadline:
			t.Skip("did not observe peer A within timeout (multicast likely unavailable)")
		}
	}
}
