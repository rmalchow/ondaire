package stream

import (
	"log/slog"
	"net"
	"net/netip"
	"sync"
)

// Handler receives the full datagram (header+payload) and the sender's address.
// pkt aliases the read buffer and is only valid for the duration of the call;
// handlers that retain bytes must copy.
type Handler func(pkt []byte, from netip.AddrPort)

// Mux multiplexes the STREAM_PORT UDP socket by packet type (§8.4). One Mux per
// node. Handlers are registered before/after Run and run on the single read
// goroutine — they must not block (hand off to a channel if needed).
type Mux struct {
	conn     *net.UDPConn
	mu       sync.RWMutex // guards handlers only
	handlers map[byte]Handler
	log      *slog.Logger
	done     chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
}

// NewMux wraps an already-bound UDP socket (from netx.BindTCPUDP).
func NewMux(conn *net.UDPConn, log *slog.Logger) *Mux {
	if log == nil {
		log = slog.Default()
	}
	return &Mux{
		conn:     conn,
		handlers: make(map[byte]Handler),
		log:      log.With("comp", "mux"),
		done:     make(chan struct{}),
	}
}

// Register installs the handler for a packet type, replacing any prior one.
// Safe to call before or after Run.
func (m *Mux) Register(typ byte, h Handler) {
	m.mu.Lock()
	m.handlers[typ] = h
	m.mu.Unlock()
}

// WriteTo sends a pre-encoded packet to addr. Safe for concurrent use.
func (m *Mux) WriteTo(pkt []byte, addr netip.AddrPort) (int, error) {
	return m.conn.WriteToUDPAddrPort(pkt, addr)
}

// LocalAddr returns the bound UDP address (for clock follower self-dial).
func (m *Mux) LocalAddr() netip.AddrPort {
	if a, ok := m.conn.LocalAddr().(*net.UDPAddr); ok {
		return a.AddrPort()
	}
	return netip.AddrPort{}
}

// Run starts the read loop (one goroutine). Non-blocking. Safe to call once.
func (m *Mux) Run() {
	m.wg.Add(1)
	go m.loop()
}

func (m *Mux) loop() {
	defer m.wg.Done()
	buf := make([]byte, 64*1024)
	for {
		n, from, err := m.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			select {
			case <-m.done:
				return // closed by Close()
			default:
				continue // transient read error; keep looping
			}
		}
		if n < HeaderSize || buf[0] != Magic {
			continue // drop malformed / non-ensemble datagrams
		}
		typ := buf[1]
		m.mu.RLock()
		h := m.handlers[typ]
		m.mu.RUnlock()
		if h == nil {
			continue // unknown type: forward-compat drop
		}
		h(buf[:n], from)
	}
}

// Close stops the read loop and closes the socket. Safe to call once.
func (m *Mux) Close() error {
	var err error
	m.once.Do(func() {
		close(m.done)
		err = m.conn.Close()
		m.wg.Wait()
	})
	return err
}
