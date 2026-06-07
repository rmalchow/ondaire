package stream

import (
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newLoopMux binds a UDP socket on 127.0.0.1:0 and wraps it in a Mux.
func newLoopMux(t *testing.T) *Mux {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	return NewMux(conn, nil)
}

func frame(typ byte, payload []byte) []byte {
	h := Header{Magic: Magic, Type: typ}
	return h.AppendFrame(nil, payload)
}

func TestMuxDispatchByType(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()

	type got struct {
		pkt  []byte
		from netip.AddrPort
	}
	chAudio := make(chan got, 1)
	chClock := make(chan got, 1)
	m.Register(TypeAudio, func(p []byte, f netip.AddrPort) {
		cp := append([]byte(nil), p...)
		chAudio <- got{cp, f}
	})
	m.Register(TypeClockReq, func(p []byte, f netip.AddrPort) {
		cp := append([]byte(nil), p...)
		chClock <- got{cp, f}
	})
	m.Run()

	addr := m.LocalAddr()
	if _, err := m.WriteTo(frame(TypeAudio, []byte("aud")), addr); err != nil {
		t.Fatal(err)
	}
	if _, err := m.WriteTo(frame(TypeClockReq, []byte("clk")), addr); err != nil {
		t.Fatal(err)
	}

	select {
	case g := <-chAudio:
		_, p, err := DecodeFrame(g.pkt)
		if err != nil || string(p) != "aud" {
			t.Fatalf("audio payload: %q err=%v", p, err)
		}
		if !g.from.Addr().IsLoopback() {
			t.Fatalf("from not loopback: %v", g.from)
		}
	case <-time.After(time.Second):
		t.Fatal("no audio dispatch")
	}
	select {
	case g := <-chClock:
		_, p, _ := DecodeFrame(g.pkt)
		if string(p) != "clk" {
			t.Fatalf("clock payload: %q", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no clock dispatch")
	}
}

func TestMuxDropsShortAndBadMagic(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()
	var hits int32
	m.Register(TypeAudio, func([]byte, netip.AddrPort) { atomic.AddInt32(&hits, 1) })
	m.Run()
	addr := m.LocalAddr()
	m.WriteTo([]byte{0x01, 0x02}, addr)  // too short
	bad := frame(TypeAudio, []byte("x")) // good frame...
	bad[0] = 0x00                        // ...with bad magic
	m.WriteTo(bad, addr)
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("dispatched %d malformed datagrams", hits)
	}
}

func TestMuxDropsUnknownType(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()
	m.Run()
	addr := m.LocalAddr()
	// No handler registered for 0x77; must not panic.
	if _, err := m.WriteTo(frame(0x77, []byte("z")), addr); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestMuxWriteToRoundTrip(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()
	ch := make(chan []byte, 1)
	m.Register(TypeFEC, func(p []byte, _ netip.AddrPort) {
		ch <- append([]byte(nil), p...)
	})
	m.Run()
	want := []byte("parity-bytes-1234")
	m.WriteTo(frame(TypeFEC, want), m.LocalAddr())
	select {
	case pkt := <-ch:
		_, p, _ := DecodeFrame(pkt)
		if string(p) != string(want) {
			t.Fatalf("payload mismatch: %q", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no delivery")
	}
}

func TestMuxRegisterAfterRun(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()
	m.Run()
	ch := make(chan struct{}, 1)
	m.Register(TypeReconfig, func([]byte, netip.AddrPort) { ch <- struct{}{} })
	m.WriteTo(frame(TypeReconfig, nil), m.LocalAddr())
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("handler registered after Run never fired")
	}
}

func TestMuxCloseUnblocks(t *testing.T) {
	m := newLoopMux(t)
	m.Run()
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return (goroutine leak)")
	}
	// second close is safe
	_ = m.Close()
}

func TestMuxConcurrentWriteTo(t *testing.T) {
	m := newLoopMux(t)
	defer m.Close()
	m.Register(TypeAudio, func([]byte, netip.AddrPort) {})
	m.Run()
	addr := m.LocalAddr()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				m.WriteTo(frame(TypeAudio, []byte("x")), addr)
			}
		}()
	}
	wg.Wait()
}

// TestMuxUnmapsV4MappedSender locks the dual-stack canonicalization: a sender
// reported as v4-mapped IPv6 (as wildcard-bound sockets do for IPv4 peers)
// must reach handlers as plain IPv4, or address-gated consumers (the
// subscriber client) silently drop every packet. Regression: LAN playback was
// silent on wildcard binds while loopback e2e (v4-only sockets) passed.
func TestMuxUnmapsV4MappedSender(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	m := NewMux(conn, nil)
	got := make(chan netip.AddrPort, 1)
	m.Register(0x7f, func(_ []byte, from netip.AddrPort) {
		select {
		case got <- from:
		default:
		}
	})
	m.Run()
	defer m.Close()

	// Hand the read loop a synthetic v4-mapped sender by writing from a second
	// socket and asserting the delivered addr is unmapped regardless of how the
	// kernel reports it.
	src, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	pkt := []byte{Magic, 0x7f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := src.WriteToUDPAddrPort(pkt, m.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	select {
	case from := <-got:
		if from.Addr().Is4In6() {
			t.Fatalf("handler saw v4-mapped sender %v; mux must unmap", from)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("packet not dispatched")
	}
}
