package playback

import (
	"net/netip"
	"reflect"
	"testing"

	"ensemble/internal/contracts"
	"ensemble/internal/id"
	"ensemble/internal/stream"
)

// --- fakes ------------------------------------------------------------------

type fakeClock struct {
	dst netip.AddrPort
	gen uint32
	n   int
}

func (c *fakeClock) SetMaster(dst netip.AddrPort, gen uint32) {
	c.dst, c.gen = dst, gen
	c.n++
}

type fakeSub struct {
	subAddr netip.AddrPort
	subGen  uint32
	subTr   stream.Transport
	subs    int
	unsubs  int
}

func (s *fakeSub) Subscribe(addr netip.AddrPort, gen uint32, t stream.Transport) error {
	s.subAddr, s.subGen, s.subTr = addr, gen, t
	s.subs++
	return nil
}
func (s *fakeSub) Unsubscribe() { s.unsubs++ }

type fakeSink struct {
	resetGen uint32
	resets   int
	disarms  int
	gain     float64
	gainSet  int
	delayNs  int64
	stats    contracts.SinkStats
}

func (s *fakeSink) Push(gen uint32, seq uint64, pts int64, payload []byte) {}
func (s *fakeSink) Reset(gen uint32)                                       { s.resetGen = gen; s.resets++ }
func (s *fakeSink) Disarm()                                                { s.disarms++ }
func (s *fakeSink) Stats() contracts.SinkStats                             { return s.stats }
func (s *fakeSink) SetGain(g float64)                                      { s.gain = g; s.gainSet++ }
func (s *fakeSink) SetDelayOffset(nanos int64)                             { s.delayNs = nanos }
func (s *fakeSink) Close() error                                           { return nil }

func newTestPlayer() (*localPlayer, *fakeClock, *fakeSub, *fakeSink) {
	clk := &fakeClock{}
	sub := &fakeSub{}
	snk := &fakeSink{}
	p := NewLocal(Config{Self: id.New(), Clock: clk, Sub: sub, Sink: snk}).(*localPlayer)
	return p, clk, sub, snk
}

// --- tests ------------------------------------------------------------------

func TestAttachPointsClockArmsSinkSubscribes(t *testing.T) {
	p, clk, sub, snk := newTestPlayer()
	src := netip.MustParseAddrPort("10.0.0.5:9200")
	clkAddr := netip.MustParseAddrPort("10.0.0.5:9090")
	p.Attach(Attach{Source: src, Clock: clkAddr, Gen: 7, Transport: stream.TransportTCP})

	if clk.dst != clkAddr || clk.gen != 7 || clk.n != 1 {
		t.Fatalf("clock: got dst=%v gen=%d n=%d", clk.dst, clk.gen, clk.n)
	}
	if snk.resets != 1 || snk.resetGen != 7 {
		t.Fatalf("sink reset: resets=%d gen=%d", snk.resets, snk.resetGen)
	}
	if sub.subs != 1 || sub.subAddr != src || sub.subGen != 7 || sub.subTr != stream.TransportTCP {
		t.Fatalf("subscribe: subs=%d addr=%v gen=%d tr=%v", sub.subs, sub.subAddr, sub.subGen, sub.subTr)
	}
	if !p.playing {
		t.Fatal("playing should be true after Attach")
	}
}

func TestDetachUnsubscribesAndDisarmsButKeepsClock(t *testing.T) {
	p, clk, sub, snk := newTestPlayer()
	p.Attach(Attach{Source: netip.MustParseAddrPort("10.0.0.5:9200"), Clock: netip.MustParseAddrPort("10.0.0.5:9090"), Gen: 1})
	clkCalls := clk.n
	p.Detach()
	if sub.unsubs != 1 || snk.disarms != 1 {
		t.Fatalf("detach: unsubs=%d disarms=%d", sub.unsubs, snk.disarms)
	}
	if clk.n != clkCalls {
		t.Fatal("Detach must not re-point the clock")
	}
	if p.playing {
		t.Fatal("playing should be false after Detach")
	}
}

func TestSyncPointsClockOnly(t *testing.T) {
	p, clk, sub, snk := newTestPlayer()
	clkAddr := netip.MustParseAddrPort("10.0.0.9:9090")
	p.Sync(clkAddr, 3)
	if clk.dst != clkAddr || clk.gen != 3 || clk.n != 1 {
		t.Fatalf("sync clock: dst=%v gen=%d n=%d", clk.dst, clk.gen, clk.n)
	}
	if sub.subs != 0 || sub.unsubs != 0 || snk.resets != 0 || snk.disarms != 0 {
		t.Fatal("Sync must touch only the clock")
	}
}

func TestSetVolumeAndMute(t *testing.T) {
	p, _, _, snk := newTestPlayer()
	p.SetVolume(50, false)
	if snk.gain != 0.5 {
		t.Fatalf("gain = %v, want 0.5", snk.gain)
	}
	p.SetVolume(80, true) // muted: gain 0, but remember 80
	if snk.gain != 0 {
		t.Fatalf("muted gain = %v, want 0", snk.gain)
	}
	p.SetVolume(80, false) // unmute restores 80%
	if snk.gain != 0.8 {
		t.Fatalf("unmuted gain = %v, want 0.8", snk.gain)
	}
	p.SetVolume(200, false) // clamps to 100
	if snk.gain != 1.0 {
		t.Fatalf("clamped gain = %v, want 1.0", snk.gain)
	}
}

func TestSetDelayConvertsMsToNs(t *testing.T) {
	p, _, _, snk := newTestPlayer()
	p.SetDelay(-30)
	if snk.delayNs != -30_000_000 {
		t.Fatalf("delayNs = %d, want -30e6", snk.delayNs)
	}
}

func TestStatusMapsSinkAndClockStats(t *testing.T) {
	clk := &fakeClock{}
	sub := &fakeSub{}
	snk := &fakeSink{stats: contracts.SinkStats{
		Played: 100, Silence: 5, LateDrop: 2, Buffered: 9, Synced: false, RatePPM: -31.25,
	}}
	self := id.New()
	p := NewLocal(Config{
		Self: self, Clock: clk, Sub: sub, Sink: snk,
		ClockStats: func() (int64, int64, bool) { return -123456, 420000, true },
	}).(*localPlayer)
	p.Attach(Attach{Clock: netip.MustParseAddrPort("10.0.0.5:9090"), Gen: 1})

	got := p.Status()
	if got.NodeID != [16]byte(self) {
		t.Fatal("NodeID mismatch")
	}
	if !got.Playing || !got.Synced { // Synced overridden true by ClockStats
		t.Fatalf("flags: playing=%v synced=%v", got.Playing, got.Synced)
	}
	if got.OffsetNs != -123456 || got.RTTNs != 420000 {
		t.Fatalf("clock fields: off=%d rtt=%d", got.OffsetNs, got.RTTNs)
	}
	if got.Played != 100 || got.Silence != 5 || got.Late != 2 || got.Buffered != 9 {
		t.Fatalf("counters: %+v", got)
	}
	if got.RatePPMx1000 != -31250 {
		t.Fatalf("ratePPMx1000 = %d, want -31250", got.RatePPMx1000)
	}
}

func TestStatusWithoutClockStatsUsesSinkSynced(t *testing.T) {
	p, _, _, snk := newTestPlayer()
	snk.stats = contracts.SinkStats{Synced: true}
	if got := p.Status(); !got.Synced {
		t.Fatal("with no ClockStats, Synced should come from the sink")
	}
	if got := p.Status(); got.OffsetNs != 0 || got.RTTNs != 0 {
		t.Fatal("with no ClockStats, clock fields should be zero")
	}
}

// localPlayer must satisfy the Player interface.
var _ Player = (*localPlayer)(nil)

func TestInterfaceShape(t *testing.T) {
	// Guard against an accidental signature drift between Player and localPlayer.
	if reflect.TypeOf((*localPlayer)(nil)).NumMethod() == 0 {
		t.Fatal("localPlayer has no methods")
	}
}
