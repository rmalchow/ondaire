// Package netx provides bind-or-increment listeners (§2) and the node's own
// interface CIDR list (§3.1). No goroutines, no shared state.
package netx

import (
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// DefaultTries is the spec's 64-attempt cap (§2).
const DefaultTries = 64

// dscpAF41 is DiffServ AF41 (DSCP 34) as the raw TOS/TCLASS byte (34<<2 =
// 0x88). Marking the UDP sockets with it maps the audio/clock/control
// datagrams into the WMM video access category (AC_VI, 802.11 UP 4) on Wi-Fi
// hops, vs the best-effort default — a shorter contention window on a busy AP
// without claiming the voice AC (AC_VO), which on the real AP starved the
// audio cadence and was reverted.
const dscpAF41 = 0x88

// setDSCP best-effort marks a UDP socket with dscpAF41 for both address
// families (a wildcard bind is a dual-stack AF_INET6 socket on Linux: IP_TOS
// covers the v4-mapped traffic, IPV6_TCLASS the native v6). Errors are
// ignored: an OS that refuses the option just leaves the traffic best-effort,
// which is exactly today's behavior.
func setDSCP(c *net.UDPConn) {
	_ = ipv4.NewPacketConn(c).SetTOS(dscpAF41)
	_ = ipv6.NewPacketConn(c).SetTrafficClass(dscpAF41)
}

// BindTCPUDP implements bind-or-increment with all-or-nothing semantics (§2).
// It tries ports base, base+1, … base+tries-1. A port is accepted only if BOTH
// the TCP listener and the UDP socket bind on that exact number; on partial
// failure both are closed and the next port is tried. host is the bind address
// ("" / "0.0.0.0" for all interfaces). Returns the bound listeners and the port
// actually used.
func BindTCPUDP(host string, base, tries int) (
	tcpLn *net.TCPListener, udpConn *net.UDPConn, port int, err error) {

	ip := parseHost(host)
	var lastErr error
	for off := 0; off < tries; off++ {
		p := base + off
		tcp, terr := net.ListenTCP("tcp", &net.TCPAddr{IP: ip, Port: p})
		if terr != nil {
			lastErr = terr
			continue
		}
		udp, uerr := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: p})
		if uerr != nil {
			// all-or-nothing: don't leak the TCP listener
			tcp.Close()
			lastErr = uerr
			continue
		}
		setDSCP(udp)
		return tcp, udp, p, nil
	}
	if tries == 1 {
		return nil, nil, 0, fmt.Errorf("netx: port %d (TCP+UDP) unavailable: %w", base, lastErr)
	}
	return nil, nil, 0, fmt.Errorf("netx: no free TCP+UDP port in [%d,%d): %w", base, base+tries, lastErr)
}

// BindTCP is the HTTP-port variant: TCP only, same increment policy.
func BindTCP(host string, base, tries int) (*net.TCPListener, int, error) {
	ip := parseHost(host)
	var lastErr error
	for off := 0; off < tries; off++ {
		p := base + off
		tcp, terr := net.ListenTCP("tcp", &net.TCPAddr{IP: ip, Port: p})
		if terr != nil {
			lastErr = terr
			continue
		}
		return tcp, p, nil
	}
	if tries == 1 {
		return nil, 0, fmt.Errorf("netx: port %d (TCP) unavailable: %w", base, lastErr)
	}
	return nil, 0, fmt.Errorf("netx: no free TCP port in [%d,%d): %w", base, base+tries, lastErr)
}

// InterfaceCIDRs returns the node's own non-loopback, up interface addresses in
// CIDR notation (e.g. "192.168.1.17/24", "fd00::5/64") for the node record
// (§3.1). Link-local and down interfaces are skipped. Empty slice on a
// loopback-only/restricted host (no error).
func InterfaceCIDRs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			out = append(out, ipnet.String())
		}
	}
	return out
}

func parseHost(host string) net.IP {
	if host == "" || host == "0.0.0.0" {
		return nil
	}
	return net.ParseIP(host)
}
