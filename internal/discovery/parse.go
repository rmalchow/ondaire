package discovery

import (
	"net"
	"net/netip"
	"strconv"
	"strings"

	"ensemble/internal/id"
)

// parseEntry converts a raw zeroconf entry into a Peer, returning ok=false for
// our own advertisement, malformed TXT, or an entry with no usable address
// (§3 / §3.1). It is pure: no network, no locking.
func parseEntry(e *zeroconfServiceEntry, self id.ID) (Peer, bool) {
	if e == nil {
		return Peer{}, false
	}

	txt := parseTXT(e.Text)

	pid, err := id.Parse(txt["id"])
	if err != nil {
		return Peer{}, false
	}
	// Self-discovery: drop our own advertisement (§3). This is the primary
	// filter, not address comparison (own/loopback IPs are unreliable).
	if pid == self {
		return Peer{}, false
	}

	gossip, ok := parsePort(txt["gossip"])
	if !ok {
		return Peer{}, false
	}
	http, ok := parsePort(txt["http"])
	if !ok {
		return Peer{}, false
	}
	stream, ok := parsePort(txt["stream"])
	if !ok {
		return Peer{}, false
	}
	source, ok := parsePort(txt["source"])
	if !ok {
		return Peer{}, false
	}

	addr, ok := chooseAddr(e.AddrIPv4, e.AddrIPv6)
	if !ok {
		return Peer{}, false
	}

	return Peer{
		ID:         pid,
		Addr:       addr,
		GossipPort: gossip,
		HTTPPort:   http,
		StreamPort: stream,
		SourcePort: source,
	}, true
}

// parseTXT turns ["k=v", ...] into a map. The first occurrence of a key wins;
// entries without '=' are ignored.
func parseTXT(text []string) map[string]string {
	m := make(map[string]string, len(text))
	for _, t := range text {
		k, v, found := strings.Cut(t, "=")
		if !found {
			continue
		}
		if _, dup := m[k]; dup {
			continue
		}
		m[k] = v
	}
	return m
}

// parsePort parses a decimal port string and validates 1..65535.
func parsePort(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// chooseAddr picks a usable responder address: IPv4 first, then global/ULA
// IPv6, skipping loopback, unspecified, and link-local (no zone handling in
// v1, §3.1). Returns ok=false if none usable.
func chooseAddr(v4, v6 []net.IP) (netip.Addr, bool) {
	for _, ip := range v4 {
		if a, ok := usableAddr(ip); ok {
			return a, true
		}
	}
	for _, ip := range v6 {
		if a, ok := usableAddr(ip); ok {
			return a, true
		}
	}
	return netip.Addr{}, false
}

// usableAddr converts a net.IP to a netip.Addr if it is a routable unicast
// address (not loopback, unspecified, or link-local).
func usableAddr(ip net.IP) (netip.Addr, bool) {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsUnspecified() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() {
		return netip.Addr{}, false
	}
	return a, true
}
