package playback

import (
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"

	"ensemble/internal/stream"
)

// --- fakes ------------------------------------------------------------------

type fakePlayer struct {
	mu       sync.Mutex
	attaches []Attach
	detaches int
	vols     []volCall
	delays   []int
	equalize []int
	channels []uint8
	caps     []capCall
	status   stream.StatusPayload
}

type volCall struct {
	pct  uint8
	mute bool
}
type capCall struct {
	id uint8
	on bool
}

func (p *fakePlayer) Attach(a Attach)             { p.mu.Lock(); p.attaches = append(p.attaches, a); p.mu.Unlock() }
func (p *fakePlayer) Detach()                     { p.mu.Lock(); p.detaches++; p.mu.Unlock() }
func (p *fakePlayer) Sync(netip.AddrPort, uint32) {}
func (p *fakePlayer) SetVolume(pct uint8, mute bool) {
	p.mu.Lock()
	p.vols = append(p.vols, volCall{pct, mute})
	p.mu.Unlock()
}
func (p *fakePlayer) SetDelay(ms int) { p.mu.Lock(); p.delays = append(p.delays, ms); p.mu.Unlock() }
func (p *fakePlayer) SetChannel(mode uint8) {
	p.mu.Lock()
	p.channels = append(p.channels, mode)
	p.mu.Unlock()
}

func (p *fakePlayer) SetEqualize(ms int) {
	p.mu.Lock()
	p.equalize = append(p.equalize, ms)
	p.mu.Unlock()
}
func (p *fakePlayer) SetCap(id uint8, on bool) {
	p.mu.Lock()
	p.caps = append(p.caps, capCall{id, on})
	p.mu.Unlock()
}
func (p *fakePlayer) Status() stream.StatusPayload { return p.status }

type writeRec struct {
	pkt []byte
	dst netip.AddrPort
}

// fakeConn implements controlConn directly.
type fakeConn struct {
	mu     sync.Mutex
	writes []writeRec
}

func (c *fakeConn) ReadFromUDPAddrPort(b []byte) (int, netip.AddrPort, error) {
	return 0, netip.AddrPort{}, nil // unused: tests call handle()/sendStatus() directly
}
func (c *fakeConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, writeRec{append([]byte(nil), b...), addr})
	c.mu.Unlock()
	return len(b), nil
}
func (c *fakeConn) SetReadDeadline(time.Time) error { return nil }

func newTestListener() (*Listener, *fakePlayer, *fakeConn) {
	pl := &fakePlayer{}
	conn := &fakeConn{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	l := &Listener{conn: conn, player: pl, log: log, done: make(chan struct{})}
	return l, pl, conn
}

// --- helpers ----------------------------------------------------------------

func attachPkt(src, clk netip.AddrPort, codec stream.Codec, tr stream.Transport, bufferMs uint16) (byte, []byte) {
	a := stream.AttachPayload{Source: src, Clock: clk, Codec: codec, Transport: tr, BufferMs: bufferMs}
	return stream.TypeAttach, a.AppendTo(nil)
}

// --- tests ------------------------------------------------------------------

func TestListenerAttachThenHeartbeatDedup(t *testing.T) {
	l, pl, _ := newTestListener()
	src := netip.MustParseAddrPort("10.0.0.5:9200")
	clk := netip.MustParseAddrPort("10.0.0.5:9090")
	ty, pkt := attachPkt(src, clk, stream.CodecOpus, stream.TransportUDP, 150)

	from := netip.MustParseAddrPort("10.0.0.5:40000")
	l.handle(ty, pkt, from)
	l.handle(ty, pkt, from) // identical re-assert (soft-state heartbeat)
	l.handle(ty, pkt, from)

	if len(pl.attaches) != 1 {
		t.Fatalf("Attach forwarded %d times, want 1 (heartbeat must dedup)", len(pl.attaches))
	}
	a := pl.attaches[0]
	if a.Source != src || a.Clock != clk || a.Gen != 0 || a.Codec != stream.CodecOpus || a.BufferMs != 150 {
		t.Fatalf("attach params: %+v", a)
	}
	// statusDst learned from the attach source.
	if l.statusDst != src {
		t.Fatalf("statusDst = %v, want %v", l.statusDst, src)
	}
}

func TestListenerReattachOnChange(t *testing.T) {
	l, pl, _ := newTestListener()
	clk := netip.MustParseAddrPort("10.0.0.5:9090")
	ty1, p1 := attachPkt(netip.MustParseAddrPort("10.0.0.5:9200"), clk, stream.CodecOpus, stream.TransportUDP, 150)
	ty2, p2 := attachPkt(netip.MustParseAddrPort("10.0.0.9:9200"), clk, stream.CodecOpus, stream.TransportUDP, 150) // new source
	l.handle(ty1, p1, netip.AddrPort{})
	l.handle(ty2, p2, netip.AddrPort{})
	if len(pl.attaches) != 2 {
		t.Fatalf("Attach forwarded %d times, want 2 (source changed)", len(pl.attaches))
	}
}

func TestListenerDetachDedup(t *testing.T) {
	l, pl, _ := newTestListener()
	ty, pkt := attachPkt(netip.MustParseAddrPort("10.0.0.5:9200"), netip.MustParseAddrPort("10.0.0.5:9090"), stream.CodecPCM, stream.TransportUDP, 150)
	l.handle(ty, pkt, netip.AddrPort{})
	l.handle(stream.TypeDetach, nil, netip.AddrPort{})
	l.handle(stream.TypeDetach, nil, netip.AddrPort{}) // repeat: no-op
	if pl.detaches != 1 {
		t.Fatalf("Detach forwarded %d times, want 1", pl.detaches)
	}
}

func TestListenerSetVolDedupAndChange(t *testing.T) {
	l, pl, _ := newTestListener()
	v := stream.SetVolPayload{VolumePct: 70, Mute: false}
	l.handle(stream.TypeSetVol, v.AppendTo(nil), netip.AddrPort{})
	l.handle(stream.TypeSetVol, v.AppendTo(nil), netip.AddrPort{}) // dedup
	l.handle(stream.TypeSetVol, (stream.SetVolPayload{VolumePct: 70, Mute: true}).AppendTo(nil), netip.AddrPort{})
	if len(pl.vols) != 2 {
		t.Fatalf("SetVolume forwarded %d times, want 2", len(pl.vols))
	}
	if pl.vols[0] != (volCall{70, false}) || pl.vols[1] != (volCall{70, true}) {
		t.Fatalf("vols = %+v", pl.vols)
	}
}

func TestListenerSetDelayDedup(t *testing.T) {
	l, pl, _ := newTestListener()
	d := stream.SetDelayPayload{DelayMs: -25}
	l.handle(stream.TypeSetDelay, d.AppendTo(nil), netip.AddrPort{})
	l.handle(stream.TypeSetDelay, d.AppendTo(nil), netip.AddrPort{}) // dedup (re-anchor would be audible)
	l.handle(stream.TypeSetDelay, (stream.SetDelayPayload{DelayMs: 10}).AppendTo(nil), netip.AddrPort{})
	if len(pl.delays) != 2 || pl.delays[0] != -25 || pl.delays[1] != 10 {
		t.Fatalf("delays = %+v, want [-25 10]", pl.delays)
	}
}

func TestListenerSetEqualizeDedup(t *testing.T) {
	l, pl, _ := newTestListener()
	e := stream.SetEqualizePayload{DelayMs: 70}
	l.handle(stream.TypeSetEq, e.AppendTo(nil), netip.AddrPort{})
	l.handle(stream.TypeSetEq, e.AppendTo(nil), netip.AddrPort{}) // dedup: re-anchor every tick would be audible
	l.handle(stream.TypeSetEq, (stream.SetEqualizePayload{DelayMs: 0}).AppendTo(nil), netip.AddrPort{})
	if len(pl.equalize) != 2 || pl.equalize[0] != 70 || pl.equalize[1] != 0 {
		t.Fatalf("equalize = %+v, want [70 0]", pl.equalize)
	}
}

func TestListenerSetCapForwarded(t *testing.T) {
	l, pl, _ := newTestListener()
	c := stream.SetCapPayload{CapID: 2, On: true}
	l.handle(stream.TypeSetCap, c.AppendTo(nil), netip.AddrPort{})
	if len(pl.caps) != 1 || pl.caps[0] != (capCall{2, true}) {
		t.Fatalf("caps = %+v", pl.caps)
	}
}

func TestListenerUnknownTypeIgnored(t *testing.T) {
	l, pl, _ := newTestListener()
	l.handle(0x7E, []byte{1, 2, 3}, netip.AddrPort{})                // unknown
	l.handle(stream.TypeAudio, make([]byte, 3840), netip.AddrPort{}) // data-plane type, wrong port
	if len(pl.attaches)+pl.detaches+len(pl.vols)+len(pl.delays)+len(pl.equalize)+len(pl.caps) != 0 {
		t.Fatal("unknown/data-plane types must not drive the player")
	}
}

func TestListenerSendStatus(t *testing.T) {
	l, pl, conn := newTestListener()
	pl.status = stream.StatusPayload{NodeID: [16]byte{0xAB}, Synced: true, Playing: true, Played: 99}
	// not attached yet → no status
	l.sendStatus()
	if len(conn.writes) != 0 {
		t.Fatal("status sent while detached")
	}
	// attach, then status flows to the source endpoint.
	src := netip.MustParseAddrPort("10.0.0.5:9200")
	ty, pkt := attachPkt(src, netip.MustParseAddrPort("10.0.0.5:9090"), stream.CodecPCM, stream.TransportUDP, 150)
	l.handle(ty, pkt, netip.AddrPort{})
	l.sendStatus()
	if len(conn.writes) != 1 {
		t.Fatalf("status writes = %d, want 1", len(conn.writes))
	}
	w := conn.writes[0]
	if w.dst != src {
		t.Fatalf("status dst = %v, want %v", w.dst, src)
	}
	h, payload, err := stream.DecodeFrame(w.pkt)
	if err != nil || h.Type != stream.TypeStatus {
		t.Fatalf("status frame: type=0x%02x err=%v", h.Type, err)
	}
	got, err := stream.DecodeStatus(payload)
	if err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if got.NodeID != [16]byte{0xAB} || !got.Synced || !got.Playing || got.Played != 99 {
		t.Fatalf("status payload mismatch: %+v", got)
	}
}

// Listener.player must accept the real localPlayer too (interface conformance).
var _ Player = (*fakePlayer)(nil)
