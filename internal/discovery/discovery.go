// Package discovery registers and browses the _ondaire._tcp mDNS service and
// emits discovered peers on a channel (§3 / §3.1). It is the mDNS half of
// discovery only: it does no gossip joining — the cluster piece (C) consumes
// the deduplicated Peer channel and joins peers itself. The dependency arrow is
// C→B, never B→C: this package imports only stdlib, internal/id, and zeroconf.
package discovery

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"ondaire/internal/id"

	"github.com/grandcat/zeroconf"
	"github.com/hashicorp/mdns"
)

// ServiceName / Domain are the fixed mDNS coordinates for ondaire (§3).
const (
	ServiceName = "_ondaire._tcp"
	Domain      = "local."
)

// Tunables (package vars so tests can shrink them).
var (
	// reEmitInterval re-emits an unchanged peer for liveness refresh (§2 rule 3).
	reEmitInterval = 30 * time.Second
	// retryInterval is the backoff between register/browse retries (§3 "always on").
	retryInterval = 5 * time.Second
	// peersBuffer is the capacity of the Peers channel (§ edge cases).
	peersBuffer = 64
	// browseWindow bounds one hashicorp/mdns query. mdns sends its PTR query
	// ONCE per window and then just listens, so a long window is (nearly) free:
	// it cuts our multicast query rate — and the response burst every query
	// solicits from all responders — by ~7× vs the old 8 s window, which
	// matters on Wi-Fi where every multicast frame goes out unacknowledged at
	// the lowest basic rate. Listening ~92% of the time (vs ~60%) also catches
	// more of the ESP32 nodes' 30 s re-announces (their only discovery path —
	// see esp32/main/mdns_adv.c), keeping idle-node re-emits well inside the
	// 90 s playback liveness TTL. A freshly booted node announces itself, so a
	// long open window HELPS cold discovery; worst-case for us being between
	// windows stays retryInterval. Close cancels the query's context, so this
	// never delays shutdown.
	browseWindow = 55 * time.Second
)

// mdnsLogger swallows hashicorp/mdns's per-packet logging (failed reads on the
// other-family socket, etc.); browseKeeper logs the meaningful browse errors.
var mdnsLogger = log.New(io.Discard, "", 0)

// mdnsEntry is the parsed view parseEntry consumes: the subset of mDNS fields we
// use, decoupled from the browse library. We browse with hashicorp/mdns because
// it assembles SRV+TXT+A across packets and actively resolves the SRV target —
// grandcat/zeroconf rebuilds its entry map per packet and silently drops any
// service whose A record arrives in a separate packet (e.g. ESP-IDF nodes), even
// though the advert is otherwise perfect (it resolves fine under avahi).
type mdnsEntry struct {
	Text     []string
	AddrIPv4 []net.IP
	AddrIPv6 []net.IP
}

// fromMDNS adapts a hashicorp/mdns entry to the subset parseEntry reads. It packs
// the TXT (InfoFields) and the single resolved IPv4/IPv6 into slices.
func fromMDNS(e *mdns.ServiceEntry) *mdnsEntry {
	m := &mdnsEntry{Text: e.InfoFields}
	if e.AddrV4 != nil {
		m.AddrIPv4 = []net.IP{e.AddrV4}
	}
	if e.AddrV6 != nil {
		m.AddrIPv6 = []net.IP{e.AddrV6}
	} else if e.AddrV6IPAddr != nil {
		m.AddrIPv6 = []net.IP{e.AddrV6IPAddr.IP}
	}
	return m
}

// Peer is one discovered ondaire node, as advertised in its mDNS TXT record
// (§3 / PLAYER §5) plus the address the responder was reached on. It is the
// unit C consumes: a Master peer is gossip-joined; a playback-only peer (D50) is
// represented as a non-gossiping member instead.
//
// A node advertises EITHER role=master (gossips; carries gossip/http/stream/source)
// OR role=playback (wire-driven; carries control + caps). A combined master+playback
// node advertises as master — its local playout is driven in-process (D61), so it is
// just a master to the network.
type Peer struct {
	ID   id.ID      // from TXT "id=" (32 hex); never Zero on a valid Peer
	Addr netip.Addr // chosen responder IP (IPv4 preferred, then IPv6)

	Master   bool   // role=master: gossips
	Playback bool   // role=playback: wire-driven, non-gossiping (D50)
	Name     string // advertised room/speaker name (masters + playback nodes, from TXT)

	// AppVersion is the peer's build version (TXT "ver="), e.g. "v0.12.1" or "dev".
	// Advertised by every role so the master/UI can spot a node running stale
	// firmware — the cheap answer to "is this Pi actually on the new wire format?".
	AppVersion string

	// Master fields (zero when !Master).
	GossipPort int // from TXT "gossip="
	HTTPPort   int // from TXT "http="
	StreamPort int // from TXT "stream="
	SourcePort int // from TXT "source=" (§8.7 SOURCE_PORT)

	// Playback fields (zero/empty when !Playback).
	ControlPort int  // from TXT "control=" (master→playback commands, D58)
	Caps        Caps // advertised capabilities (PLAYER §5)
}

// Caps are a playback node's advertised capabilities (PLAYER §5). They inform
// the master/operator (e.g. a UI warning when assigning a pcm-only speaker to an
// opus group); they never gate membership or fan-out (D51).
type Caps struct {
	Codecs         []string // decodable codecs, preference order ("codecs=pcm,opus")
	MaxRate        int      // max output sample rate ("rate=48000")
	HWVolume       bool     // hardware volume control present ("hwvol=1")
	FixedDelayMs   int      // unchangeable output delay, ms ("delayms=30")
	CanReportQueue bool     // can report output-queue depth → runs the servo ("queue=1")
	Input          bool     // has line-in capture ("input=1")
}

// GossipAddrPort is the address C dials to join the peer's gossip cluster (§3).
// Only meaningful for a Master peer.
func (p Peer) GossipAddrPort() netip.AddrPort {
	return netip.AddrPortFrom(p.Addr, uint16(p.GossipPort))
}

// PlaybackOnly reports whether this peer is a wire-driven, non-gossiping playback
// node (D50): the master ingests it as a member and drives it over the control
// plane rather than gossip-joining it.
func (p Peer) PlaybackOnly() bool {
	return p.Playback && !p.Master && p.ControlPort > 0
}

// Config carries this node's own advertised identity. A node advertises as a
// master (gossip/http/stream/source) when Master is set, else as a playback node
// (control + caps). Master and Playback come from the node's role (D49).
type Config struct {
	ID       id.ID  // this node's immutable node ID (§1)
	Instance string // mDNS instance name; use ID.String() for uniqueness

	// HostIP, when set, is the single IP advertised in the mDNS A/AAAA record
	// (§3.1). It comes from an explicit --host / ONDAIRE_HOST. Empty => let
	// zeroconf enumerate every multicast interface, which on a host-networked
	// container picks up docker-bridge addresses (e.g. 172.18.0.1) that peers
	// can't route to. Pinning it makes ONDAIRE_HOST authoritative for discovery,
	// not just for binding.
	HostIP string

	Master   bool   // advertise role=master + its ports
	Playback bool   // (when !Master) advertise role=playback + control + caps
	Name     string // advertised room/speaker name (playback-only advert)
	Version  string // build version advertised in TXT "ver=" (main.version)

	GossipPort int // actually-bound ports (§2 bind-or-increment result)
	HTTPPort   int
	StreamPort int
	SourcePort int // actually-bound SOURCE_PORT (§2/§8.7)

	ControlPort int  // bound CONTROL_PORT (playback-only advert, D58)
	Caps        Caps // advertised capabilities (playback-only advert)

	Log *slog.Logger // optional; defaults to slog.Default() with comp=discovery
}

// seenEntry is per-peer dedup state (§ control flow).
type seenEntry struct {
	peer     Peer      // last emitted value
	lastSeen time.Time // last time we OBSERVED this peer (browse hit)
	emitted  time.Time // last time we EMITTED on the channel
}

// Discovery registers this node over mDNS and continuously browses for peers,
// emitting deduplicated Peer events. One per node. Zero-value is not usable;
// construct with New.
type Discovery struct {
	cfg   Config
	log   *slog.Logger
	peers chan Peer

	// browseFn runs one bounded browse window, emitting peers; a test seam,
	// defaults to realBrowse (hashicorp/mdns). browseKeeper loops it.
	browseFn func(context.Context) error

	mu     sync.Mutex          // guards seen + server + closed
	seen   map[id.ID]seenEntry // dedup / throttle state, keyed by peer ID
	server *zeroconf.Server    // mDNS registration handle (for Shutdown)
	closed bool
	drops  uint64 // non-blocking sends dropped on a full buffer (diagnostic)

	ctx    context.Context // cancelled by Close
	cancel context.CancelFunc
	wg     sync.WaitGroup // browse + register-refresh goroutines
}

// New constructs a Discovery. It does not touch the network; call Run to start.
func New(cfg Config) *Discovery {
	if cfg.Log == nil {
		cfg.Log = slog.Default().With("comp", "discovery")
	}
	if cfg.Instance == "" {
		cfg.Instance = cfg.ID.String()
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Discovery{
		cfg:    cfg,
		log:    cfg.Log,
		peers:  make(chan Peer, peersBuffer),
		seen:   make(map[id.ID]seenEntry),
		ctx:    ctx,
		cancel: cancel,
	}
	d.browseFn = d.realBrowse
	return d
}

// Run registers the mDNS service and starts the continuous browse loop in the
// background. It is non-blocking. A registration error is logged and retried
// inside Run's goroutines, not returned, so a transient mDNS failure never
// aborts node startup (§3: "both always on"). Run is called once; calling it
// after Close is a no-op.
func (d *Discovery) Run() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	d.wg.Add(2)
	go d.registerKeeper()
	go d.browseKeeper()
}

// Peers returns the receive end of the deduplicated peer-event channel. C ranges
// over it. The channel is closed when Close completes.
func (d *Discovery) Peers() <-chan Peer {
	return d.peers
}

// Close stops browsing, unregisters the mDNS service, waits for goroutines, and
// closes the Peers channel. Idempotent; safe to call once concurrently.
func (d *Discovery) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	server := d.server
	d.mu.Unlock()

	d.cancel()
	if server != nil {
		server.Shutdown()
	}
	d.wg.Wait()
	// Under d.mu because a browse worker can outlive browseKeeper by up to one
	// window (see realBrowse) — maybeEmit's closed-check + send hold the same
	// mutex, so a straggler either emits before this close or sees closed and
	// drops; it can never send on the closed channel.
	d.mu.Lock()
	close(d.peers)
	d.mu.Unlock()
	return nil
}

// txtRecords builds the advertised TXT key=value strings (§3 / PLAYER §5).
// A master advertises its four ports; a (non-master) playback node advertises its
// control port + capabilities. The `role` key is always present.
func txtRecords(cfg Config) []string {
	recs := []string{
		"id=" + cfg.ID.String(),
		"role=" + advertRole(cfg),
		"ver=" + cfg.Version, // build version (every role) so the UI can flag stale nodes
	}
	if playbackOnly(cfg) {
		// Playback-only advert (D50/D51): control + capabilities, no gossip.
		return append(recs,
			"control="+strconv.Itoa(cfg.ControlPort),
			"name="+cfg.Name,
			"codecs="+strings.Join(cfg.Caps.Codecs, ","),
			"rate="+strconv.Itoa(cfg.Caps.MaxRate),
			"hwvol="+boolBit(cfg.Caps.HWVolume),
			"delayms="+strconv.Itoa(cfg.Caps.FixedDelayMs),
			"queue="+boolBit(cfg.Caps.CanReportQueue),
			"input="+boolBit(cfg.Caps.Input),
		)
	}
	// Master advert (the default for a combined node and any legacy/zero Config).
	// `name` lets a mDNS-only client (the Android companion app) label masters in its
	// picker without an extra /api/status round-trip — matching the playback advert.
	recs = append(recs,
		"name="+cfg.Name,
		"gossip="+strconv.Itoa(cfg.GossipPort),
		"http="+strconv.Itoa(cfg.HTTPPort),
		"stream="+strconv.Itoa(cfg.StreamPort),
		"source="+strconv.Itoa(cfg.SourcePort),
	)
	// A COMBINED node (Master AND Playback, D61) also plays via the control plane,
	// so it advertises its control port alongside the master ports. Peers learn the
	// full node's control endpoint from gossip too, but advertising it keeps the
	// mDNS view consistent for mDNS-only clients.
	if cfg.Playback && cfg.ControlPort > 0 {
		recs = append(recs, "control="+strconv.Itoa(cfg.ControlPort))
	}
	return recs
}

// playbackOnly reports whether the node advertises as a wire-driven playback node:
// it has the playback role and NOT the master role. Everything else (combined,
// master-only, zero/legacy Config) advertises as a master (D61).
func playbackOnly(cfg Config) bool {
	return cfg.Playback && !cfg.Master
}

// advertRole renders the mDNS role from the advert kind.
func advertRole(cfg Config) string {
	if playbackOnly(cfg) {
		return "playback"
	}
	return "master"
}

func boolBit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// srvPort is the (informational) mDNS SRV port: a master's HTTP port, else a
// playback node's control port, else any bound port — never 0 (zeroconf rejects
// "Missing port"). The authoritative ports are always in TXT (D4).
func srvPort(cfg Config) int {
	for _, p := range []int{cfg.HTTPPort, cfg.ControlPort, cfg.StreamPort, cfg.GossipPort, cfg.SourcePort} {
		if p > 0 {
			return p
		}
	}
	return 1 // last-resort placeholder; TXT carries the real ports
}

// proxyHost is the host name for the A/AAAA record when registering with an
// explicit HostIP. zeroconf.RegisterProxy rejects an empty host, so fall back to
// a fixed label if the OS hostname is unavailable; only the IP it points at is
// functionally significant (peers read AddrIPv4 from the response directly).
func proxyHost() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "ondaire"
}

// registerKeeper registers the mDNS service, retrying on failure until ctx is
// done. zeroconf's Server self-maintains announcements once registered, so this
// goroutine mostly sleeps after the first success; it exists so a *failed*
// initial registration heals (§3).
func (d *Discovery) registerKeeper() {
	defer d.wg.Done()

	txt := txtRecords(d.cfg)
	for {
		if d.ctx.Err() != nil {
			return
		}
		// Advertised SRV port is informational only — real ports are in TXT (D4) —
		// but zeroconf rejects port 0 ("Missing port"). A master advertises its HTTP
		// port; a playback-only node (no HTTP) falls back to its control port.
		// nil ifaces => listen on all multicast-capable interfaces either way.
		//
		// With an explicit HostIP, register a proxy that advertises exactly that
		// A/AAAA record instead of letting zeroconf enumerate every interface — on a
		// host-networked container the latter publishes a docker-bridge address peers
		// can't reach (§3.1). Without it, fall back to the all-interface registration.
		var srv *zeroconf.Server
		var err error
		if d.cfg.HostIP != "" {
			srv, err = zeroconf.RegisterProxy(d.cfg.Instance, ServiceName, Domain, srvPort(d.cfg), proxyHost(), []string{d.cfg.HostIP}, txt, nil)
		} else {
			srv, err = zeroconf.Register(d.cfg.Instance, ServiceName, Domain, srvPort(d.cfg), txt, nil)
		}
		if err == nil {
			d.mu.Lock()
			if d.closed {
				// Raced with Close: tear the fresh registration down.
				d.mu.Unlock()
				srv.Shutdown()
				return
			}
			d.server = srv
			d.mu.Unlock()
			d.log.Info("mDNS registered", "instance", d.cfg.Instance)
			d.reannounceLoop(srv, txt)
			return
		}
		d.log.Warn("mDNS register failed, retrying", "err", err)
		if !d.sleep(retryInterval) {
			return
		}
	}
}

// reannounceLoop periodically re-emits the mDNS announcement so a peer stays fresh
// between the responder's sparse RFC6762 bursts (a player node that no longer
// follows anyone still announces, so the master's browse keeps seeing it and
// liveness holds). SetText re-announces the TXT with cache-flush — no Shutdown, so
// no "goodbye"/gap. Complements the control-plane liveness poll (D60). Blocks until
// ctx is done.
func (d *Discovery) reannounceLoop(srv *zeroconf.Server, txt []string) {
	for d.sleep(reEmitInterval) {
		srv.SetText(txt)
	}
}

// browseKeeper runs browseOnce in a loop, restarting on error until ctx is done
// (§3 restart-on-error).
func (d *Discovery) browseKeeper() {
	defer d.wg.Done()

	for {
		if d.ctx.Err() != nil {
			return
		}
		if err := d.browseFn(d.ctx); err != nil && d.ctx.Err() == nil {
			d.log.Warn("mDNS browse error, restarting", "err", err)
		}
		if d.ctx.Err() != nil {
			return
		}
		if !d.sleep(retryInterval) {
			return
		}
	}
}

// realBrowse runs one bounded hashicorp/mdns query window, draining entries
// through parseEntry + maybeEmit. It returns nil on a normal window end or on
// ctx cancellation (Close) — browseKeeper re-queries on the next loop, and
// maybeEmit's dedup throttles the resulting re-emits.
//
// The query runs on a worker goroutine because hashicorp/mdns's receive loop
// waits on its Timeout only, never the ctx: cancellation closes its sockets
// but QueryContext still returns only at the window boundary. Waiting for it
// inline would hold Close hostage for up to a full browseWindow. On ctx
// cancellation we return immediately and let the worker (idle — its sockets
// are gone) unwind at the boundary; its stragglers are safe because maybeEmit
// closed-checks and sends under d.mu, the same mutex Close closes d.peers
// under.
func (d *Discovery) realBrowse(ctx context.Context) error {
	entries := make(chan *mdns.ServiceEntry, 16)
	go func() {
		for e := range entries {
			if peer, ok := parseEntry(fromMDNS(e), d.cfg.ID); ok {
				d.maybeEmit(peer)
			}
		}
	}()

	qctx, cancel := context.WithTimeout(ctx, browseWindow)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := mdns.QueryContext(qctx, &mdns.QueryParam{
			Service: ServiceName,
			Domain:  strings.TrimSuffix(Domain, "."), // hashicorp wants "local", not "local."
			Timeout: browseWindow,
			Entries: entries,
			Logger:  mdnsLogger,
		})
		close(entries) // hashicorp/mdns does not close the caller's channel
		done <- err
	}()
	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil // window ended — not a failure
		}
		return err
	case <-ctx.Done():
		return nil // shutting down; the worker self-terminates at the window boundary
	}
}

// maybeEmit applies the three-rule dedup test (§2) and does a non-blocking send
// on the peers channel. The send happens UNDER d.mu (it can't block — buffered
// channel + select-default): pairing it with the closed-check makes it safe
// against Close closing the channel, which matters because a browse worker can
// outlive browseKeeper by up to one window (see realBrowse).
func (d *Discovery) maybeEmit(peer Peer) {
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	prev, known := d.seen[peer.ID]

	emit := !known ||
		changed(prev.peer, peer) ||
		now.Sub(prev.emitted) >= reEmitInterval

	prev.lastSeen = now
	if emit {
		prev.peer = peer
		prev.emitted = now
	}
	d.seen[peer.ID] = prev

	if !emit {
		return
	}

	select {
	case d.peers <- peer:
	default:
		d.drops++
		d.log.Warn("peers channel full, dropping event (will re-offer)", "peer", peer.ID, "drops", d.drops)
	}
}

// changed reports whether any material field differs between two Peers with the
// same ID (§2 rule 2).
func changed(a, b Peer) bool {
	return a.Addr != b.Addr ||
		a.Master != b.Master || a.Playback != b.Playback ||
		a.GossipPort != b.GossipPort ||
		a.HTTPPort != b.HTTPPort ||
		a.StreamPort != b.StreamPort ||
		a.SourcePort != b.SourcePort ||
		a.ControlPort != b.ControlPort ||
		a.Name != b.Name ||
		capsChanged(a.Caps, b.Caps)
}

// capsChanged reports whether two advertised capability sets differ.
func capsChanged(a, b Caps) bool {
	return !slices.Equal(a.Codecs, b.Codecs) ||
		a.MaxRate != b.MaxRate ||
		a.HWVolume != b.HWVolume ||
		a.FixedDelayMs != b.FixedDelayMs ||
		a.CanReportQueue != b.CanReportQueue ||
		a.Input != b.Input
}

// sleep waits d or returns false early if ctx is cancelled.
func (d *Discovery) sleep(dur time.Duration) bool {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-d.ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
