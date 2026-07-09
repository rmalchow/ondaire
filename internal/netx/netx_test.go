package netx

import (
	"net"
	"strings"
	"testing"

	"golang.org/x/net/ipv4"
)

const lo = "127.0.0.1"

// freePort finds a port number that is currently free on BOTH TCP and UDP on
// loopback, then releases it. Callers use it as a fixed `base` (port 0 cannot
// be used as a base: each socket would get a different ephemeral number).
// The small rebind race is acceptable for a single-process test.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		tcp, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(lo), Port: 0})
		if err != nil {
			t.Fatalf("probe tcp: %v", err)
		}
		p := tcp.Addr().(*net.TCPAddr).Port
		udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(lo), Port: p})
		tcp.Close()
		if err != nil {
			continue // UDP busy at p; try another
		}
		udp.Close()
		return p
	}
	t.Fatal("could not find a free TCP+UDP port")
	return 0
}

func TestBindTCPUDPBasePort(t *testing.T) {
	base := freePort(t)
	tcp, udp, port, err := BindTCPUDP(lo, base, 64)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer tcp.Close()
	defer udp.Close()
	if port != base {
		t.Fatalf("port = %d, want free base %d", port, base)
	}
	if tcp.Addr().(*net.TCPAddr).Port != port {
		t.Fatal("tcp port mismatch")
	}
	if udp.LocalAddr().(*net.UDPAddr).Port != port {
		t.Fatal("udp port mismatch")
	}
}

func TestBindTCPUDPIncrements(t *testing.T) {
	// occupy a base port on both protocols
	tcp, udp, base, err := BindTCPUDP(lo, freePort(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	defer udp.Close()

	tcp2, udp2, port2, err := BindTCPUDP(lo, base, 64)
	if err != nil {
		t.Fatalf("increment bind: %v", err)
	}
	defer tcp2.Close()
	defer udp2.Close()
	if port2 <= base {
		t.Fatalf("did not increment past occupied base: port2=%d base=%d", port2, base)
	}
}

func TestBindTCPUDPSamePortBothProtos(t *testing.T) {
	tcp, udp, _, err := BindTCPUDP(lo, freePort(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	defer udp.Close()
	if tcp.Addr().(*net.TCPAddr).Port != udp.LocalAddr().(*net.UDPAddr).Port {
		t.Fatal("tcp and udp ended up on different ports")
	}
}

func TestBindTCPUDPAllOrNothing(t *testing.T) {
	// occupy only UDP at a base port
	base := freePort(t)
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(lo), Port: base})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()

	// base must be skipped because UDP is taken; we should get base+1+
	tcp2, udp2, port2, err := BindTCPUDP(lo, base, 64)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer tcp2.Close()
	defer udp2.Close()
	if port2 == base {
		t.Fatal("bound the base port whose UDP was occupied")
	}
	// the TCP listener at base must NOT have been leaked: we can bind it now.
	tcpBase, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(lo), Port: base})
	if err != nil {
		t.Fatalf("base TCP was leaked (could not rebind): %v", err)
	}
	tcpBase.Close()
}

func TestBindTCPUDPExhausted(t *testing.T) {
	tcp, udp, base, err := BindTCPUDP(lo, freePort(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	defer udp.Close()
	_, _, _, err = BindTCPUDP(lo, base, 1) // only the occupied base, tries=1
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	// tries=1 (a pinned port) reports the exact port unavailable, not a range.
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error missing single-port hint: %v", err)
	}
}

func TestBindTCPHTTP(t *testing.T) {
	tcp, port, err := BindTCP(lo, freePort(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	// UDP on the same number must still be free (BindTCP is TCP only).
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(lo), Port: port})
	if err != nil {
		t.Fatalf("BindTCP wrongly reserved UDP: %v", err)
	}
	udp.Close()
}

// TestBindTCPUDPMarksDSCP pins the WMM marking: the UDP socket must carry
// DSCP EF so the audio/clock/control datagrams ride the voice/video access
// category on Wi-Fi hops instead of best-effort.
func TestBindTCPUDPMarksDSCP(t *testing.T) {
	tcp, udp, _, err := BindTCPUDP(lo, freePort(t), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	defer udp.Close()
	tos, err := ipv4.NewPacketConn(udp).TOS()
	if err != nil {
		t.Skipf("TOS readback unsupported here: %v", err)
	}
	if tos != dscpEF {
		t.Fatalf("TOS = %#x, want %#x (DSCP EF)", tos, dscpEF)
	}
}

func TestInterfaceCIDRsShape(t *testing.T) {
	cidrs := InterfaceCIDRs() // may be empty in restricted env; must not panic
	for _, c := range cidrs {
		ip, _, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("not a CIDR: %q (%v)", c, err)
		}
		if ip.IsLoopback() {
			t.Fatalf("loopback leaked: %q", c)
		}
	}
}
