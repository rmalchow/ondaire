package source

import (
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"ondaire/internal/stream"
)

// registry holds the live subscribers, keyed by their OBSERVED source address
// (§8.7: UDP audio flows back to the addr the HELLO came from; TCP audio rides
// the accepted conn). Guarded by Server.mu (no own lock).
type registry struct {
	subs map[netip.AddrPort]*subscriber
}

func newRegistry() registry {
	return registry{subs: make(map[netip.AddrPort]*subscriber)}
}

// subscriber is one live destination.
type subscriber struct {
	addr     netip.AddrPort // UDP: HELLO source addr; TCP: RemoteAddr
	tr       stream.Transport
	conn     net.Conn    // TCP only (nil for UDP)
	lastSeen time.Time   // last HELLO/RESTART; expiry at +ttl
	wmu      sync.Mutex  // TCP: serializes writes to conn (writer goroutine + prime)
	dead     atomic.Bool // TCP write error observed; skip on fan-out (read off the writer goroutine)
	priming  bool        // a prime burst is catching up to the live edge;
	//                     excluded from live fan-out until then (Server.mu)

	// TCP async fan-out (D13): the release goroutine enqueues copied packets onto
	// sendCh; a dedicated per-sub writer goroutine (Server.startTCPWriter) owns the
	// blocking conn write, so a slow/wedged subscriber can never stall H's release
	// ticker or serialize behind Server.mu. A full queue ⇒ drop+count the frame (the
	// writer is behind); a genuinely wedged conn is retired by writeTCP's deadline.
	// Both nil for UDP (UDP fan-out is a non-blocking sendto, already cadence-safe).
	sendCh   chan []byte
	wdone    chan struct{}
	stopOnce sync.Once
}

// upsert records a HELLO from addr. Returns (sub, isNew): isNew is true on a
// previously-unknown addr (Connects++). Refreshes lastSeen otherwise.
func (r *registry) upsert(addr netip.AddrPort, t stream.Transport, conn net.Conn, now time.Time) (sub *subscriber, isNew bool) {
	if s, ok := r.subs[addr]; ok {
		s.lastSeen = now
		return s, false
	}
	s := &subscriber{addr: addr, tr: t, conn: conn, lastSeen: now}
	if t == stream.TransportTCP {
		s.sendCh = make(chan []byte, tcpSendQueue)
		s.wdone = make(chan struct{})
	}
	r.subs[addr] = s
	return s, true
}

// get returns the subscriber for addr (RESTART/BYE lookups), or nil.
func (r *registry) get(addr netip.AddrPort) *subscriber { return r.subs[addr] }

// remove drops a subscriber (BYE, or TCP conn error/close).
func (r *registry) remove(addr netip.AddrPort) { delete(r.subs, addr) }

// expire removes subscribers whose lastSeen < now-ttl; returns the removed
// subscribers (so the caller can stop their TCP writer + close the conn outside
// the map mutation) and the addrs of every expired subscriber (for logging).
func (r *registry) expire(now time.Time, ttl time.Duration) (removed []*subscriber, expired []netip.AddrPort) {
	for addr, s := range r.subs {
		if now.Sub(s.lastSeen) > ttl {
			removed = append(removed, s)
			expired = append(expired, addr)
			delete(r.subs, addr)
		}
	}
	return removed, expired
}

// live returns a snapshot slice of current subscribers for fan-out. Subs with
// a prime burst in flight are excluded — the burst delivers their frames (in
// seq order, catching up via the ring) until it reaches the live edge, so the
// newcomer's reorder window and sink anchor on the OLDEST primed seq, never on
// an interleaved live frame.
func (r *registry) live() []*subscriber {
	out := make([]*subscriber, 0, len(r.subs))
	for _, s := range r.subs {
		if s.priming {
			continue
		}
		out = append(out, s)
	}
	return out
}

// count returns the current subscriber count.
func (r *registry) count() int { return len(r.subs) }
