package playback

import (
	"context"
	"log/slog"
	"math"
	"net/netip"
	"sync"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/id"
	"ensemble/internal/stream"
)

// defaultDriveInterval is the soft-state re-assert cadence (D58): the driver
// re-sends the desired control state every tick so a lost datagram self-heals.
const defaultDriveInterval = 1 * time.Second

// Driver is the master-side control driver (D59/D62): for each non-gossiping
// playback node assigned to the group THIS node masters, it holds a remotePlayer
// and re-asserts the desired control state. It mirrors the gossiping member's
// repointLocked exactly: ATTACH (+ volume/delay) while the group plays, DETACH when
// the group is idle or the node is unassigned. STATUS flows back independently to
// the source server (C.1), not through here.
//
// Only the master named by a playback node's assignment drives it: the driver acts
// solely on the group whose Master == self, so on a multi-master cluster each node
// is driven by exactly one master (D62).
type Driver struct {
	store    contracts.StateStore
	w        controlWriter
	log      *slog.Logger
	interval time.Duration
	delays   func() map[id.ID]RoomDelay // D65: per-node device-queue state (nil → no equalization)

	mu     sync.Mutex
	active map[id.ID]*drive // playback nodes currently driven, by id
	lastEq map[id.ID]int    // last equalization ms LOGGED per node (D65; for change-only logging)

	done chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

type drive struct {
	player Player
	dst    netip.AddrPort // the node's CONTROL_PORT endpoint
}

// RoomDelay is one playback node's output-timing state, read by the driver to
// equalize cross-room device buffering (D65). SetpointNs is the servo's calibrated
// device-queue depth — DeviceDelayNs−PhaseErrNs from STATUS, a STABLE constant once
// Calibrated (the live DeviceDelayNs swings ±10 ms and would re-anchor constantly).
// Playing mirrors the node's own STATUS flag. Built by the caller (main) from the
// source server's decoded STATUS map, so the driver needs no source-package import.
type RoomDelay struct {
	SetpointNs int64
	Calibrated bool
	Playing    bool
}

// eqQuantumMs: round equalization targets to this grid so a tiny setpoint wobble
// (e.g. a 1 ms estimate difference) doesn't flip the pushed value and re-anchor.
const eqQuantumMs = 10

// DriverConfig wires a Driver. Store is the cluster read side; W is the master's
// control-sending UDP socket (sends to nodes' CONTROL_PORTs).
type DriverConfig struct {
	Store    contracts.StateStore
	W        controlWriter
	Log      *slog.Logger
	Interval time.Duration // 0 → default 1 s
	// Delays returns each playback node's calibrated device-queue state, keyed by id
	// (D65). Wired by main from the source server's STATUS map. nil disables
	// cross-room equalization (the driver then only drives attach/volume).
	Delays func() map[id.ID]RoomDelay
}

// NewDriver builds a Driver; starts no goroutines (call Run).
func NewDriver(cfg DriverConfig) *Driver {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	iv := cfg.Interval
	if iv <= 0 {
		iv = defaultDriveInterval
	}
	return &Driver{
		store:    cfg.Store,
		w:        cfg.W,
		log:      log.With("comp", "pb-driver"),
		interval: iv,
		delays:   cfg.Delays,
		active:   make(map[id.ID]*drive),
		lastEq:   make(map[id.ID]int),
		done:     make(chan struct{}),
	}
}

// Run launches the reconcile loop (cluster changes + a heartbeat ticker). Non-blocking.
func (d *Driver) Run(ctx context.Context) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		changes := d.store.Subscribe()
		t := time.NewTicker(d.interval)
		defer t.Stop()
		d.reconcile(d.store.Snapshot())
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.done:
				return
			case <-changes:
				d.reconcile(d.store.Snapshot())
			case <-t.C:
				d.reconcile(d.store.Snapshot())
			}
		}
	}()
}

// Close stops the loop and best-effort DETACHes every node it was driving (so a
// clean master shutdown idles its remote speakers immediately rather than letting
// them starve). Idempotent.
func (d *Driver) Close() error {
	d.once.Do(func() {
		close(d.done)
		d.wg.Wait()
		d.mu.Lock()
		for id, dr := range d.active {
			dr.player.Detach()
			delete(d.active, id)
		}
		d.mu.Unlock()
	})
	return nil
}

// reconcile is one pass: drive the playback members of the group we master.
func (d *Driver) reconcile(snap contracts.Snapshot) {
	self := d.store.Self()

	byID := make(map[id.ID]contracts.NodeView, len(snap.Nodes))
	for _, n := range snap.Nodes {
		byID[n.ID] = n
	}

	// Liveness poll (D60/D61): ping EVERY node that exposes a control endpoint —
	// assigned to us or not, playing or idle — so it replies with STATUS (→ source
	// server → TouchPlaybackNode → srcSrv.statuses). controlEndpoint() returns false
	// when ControlPort==0, so a plain non-playback node is skipped. A full node now
	// carries its own ControlPort (D61), so this also polls SELF over loopback → the
	// master appears in its own sync-health. One tiny datagram per node per tick.
	for _, n := range snap.Nodes {
		if ep, ok := controlEndpoint(n); ok {
			pollStatus(d.w, ep)
		}
	}

	// The group THIS node masters (D44: group id == master id; at most one).
	var myGroup *contracts.GroupView
	for i := range snap.Groups {
		if snap.Groups[i].Master == self {
			myGroup = &snap.Groups[i]
			break
		}
	}

	// Assigned playback members + whether the group is playing.
	assigned := map[id.ID]contracts.NodeView{}
	playing := false
	var settings contracts.GroupSettings
	if myGroup != nil {
		playing = myGroup.Playback.State == "playing"
		settings = myGroup.Settings
		for _, mid := range myGroup.Members {
			// Drive any assigned member that exposes a control endpoint (D61): a
			// gossiping full node (incl. SELF, driven over loopback) OR a
			// non-gossiping playback node. controlEndpoint() gates on ControlPort.
			if nv, ok := byID[mid]; ok {
				if _, cok := controlEndpoint(nv); cok {
					assigned[mid] = nv
				}
			}
		}
	}

	source, clock, epOK := selfEndpoints(byID[self])

	d.mu.Lock()
	defer d.mu.Unlock()

	// DETACH + forget nodes no longer assigned to us.
	for nid, dr := range d.active {
		if _, still := assigned[nid]; !still {
			dr.player.Detach()
			delete(d.active, nid)
			delete(d.lastEq, nid)
			d.log.Info("playback node unassigned: detached", "id", nid)
		}
	}

	// Drive each assigned node (soft-state: re-assert every tick).
	for nid, nv := range assigned {
		ep, ok := controlEndpoint(nv)
		if !ok {
			continue // no usable control endpoint yet
		}
		dr := d.active[nid]
		if dr == nil || dr.dst != ep {
			dr = &drive{player: NewRemote(d.w, ep, d.log), dst: ep}
			d.active[nid] = dr
		}
		// Configuration is the NODE's state, NOT a property of playback (D54): assert
		// volume + channel every tick whether or not the group is playing, so a
		// volume/channel change takes effect immediately even on an idle group. The
		// listener dedups, so a steady value re-applies only once.
		dr.player.SetVolume(volPct(nv.Volume), false)
		dr.player.SetChannel(chanModeByte(nv.Channel)) // dual-mono select; sink dedups
		// Output delay is the NODE's property (its fixed device latency), set at
		// startup from node.json and persisted there. The master must NOT push it
		// routinely — that would clobber the node's own value with the proxied
		// record's default (0), which is exactly the regression that broke sync.
		// Calibration is the one exception (a deliberate, measured one-shot) and
		// rides its own path, not this heartbeat.
		if playing && epOK {
			dr.player.Attach(Attach{
				Source:    source,
				Clock:     clock,
				Codec:     stream.ParseCodec(settings.Codec),
				Transport: stream.ParseTransport(settings.Transport),
				BufferMs:  settings.BufferMs,
			})
		} else {
			// Group idle (or endpoints unknown): keep the node idle (DETACH is the
			// authoritative, idempotent idle form) — but volume/channel above still
			// applied, so the node is correctly configured for its next session.
			dr.player.Detach()
		}
	}

	if playing && epOK {
		d.equalizeLocked(assigned)
	}
}

// equalizeLocked pushes cross-room device-buffer equalization (D65). For each
// playing room it reads the STABLE calibrated setpoint (DeviceDelayNs−PhaseErrNs),
// finds the slowest, and tells every faster room to DELAY by (max−own) so all
// speaker_times align. Soft-state: re-asserted every tick for loss recovery; the
// node-side listener dedups, so a steady target re-anchors only once.
//
// Gated on EVERY assigned room being ready (calibrated, or reporting no device
// queue) so the max is final — acting on a provisional max would re-anchor again
// when a slower room finishes settling. Targets are quantized (eqQuantumMs) so a
// 1 ms setpoint wobble doesn't flip the pushed value. Caller holds d.mu.
func (d *Driver) equalizeLocked(assigned map[id.ID]contracts.NodeView) {
	if d.delays == nil || len(assigned) < 2 {
		return // nothing to equalize against (or telemetry not wired)
	}
	rd := d.delays()
	var maxSp int64
	for nid := range assigned {
		r, ok := rd[nid]
		if !ok || !r.Playing {
			return // a room hasn't reported playing STATUS yet: wait
		}
		if r.SetpointNs > 0 && !r.Calibrated {
			return // a device-bearing room is still settling: max not final yet
		}
		if r.SetpointNs > maxSp {
			maxSp = r.SetpointNs
		}
	}
	for nid, dr := range d.active {
		r := rd[nid]
		want := maxSp - r.SetpointNs
		if want < 0 {
			want = 0
		}
		wantMs := int((want + eqQuantumMs/2*1_000_000) / (eqQuantumMs * 1_000_000) * eqQuantumMs)
		// Re-assert every tick (soft-state loss recovery); the node-side listener
		// dedups the re-anchor. Log only on a real change so the operator sees the
		// applied compensation without per-tick spam.
		if last, ok := d.lastEq[nid]; !ok || last != wantMs {
			d.lastEq[nid] = wantMs
			d.log.Info("room equalized", "id", nid, "equalizeMs", wantMs, "setpointMs", r.SetpointNs/1_000_000, "refMs", maxSp/1_000_000)
		}
		dr.player.SetEqualize(wantMs)
	}
}

// pollStatus sends a zero-payload STATUS_REQ to a playback node's control port; the
// node answers with STATUS to this socket (D60 liveness poll). Best-effort.
func pollStatus(w controlWriter, dst netip.AddrPort) {
	h := stream.Header{Magic: stream.Magic, Type: stream.TypeStatusReq}
	pkt := h.AppendFrame(make([]byte, 0, stream.HeaderSize), nil)
	_, _ = w.WriteToUDPAddrPort(pkt, dst)
}

// selfEndpoints derives the master's own source + clock endpoints to put in ATTACH,
// from its NodeView (a reachable self IP + the bound SOURCE/STREAM ports). ok=false
// until the record carries a usable address and ports.
func selfEndpoints(self contracts.NodeView) (source, clock netip.AddrPort, ok bool) {
	ip, ok := firstHost(self.Addrs)
	if !ok || self.SourcePort == 0 || self.StreamPort == 0 {
		return netip.AddrPort{}, netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ip, uint16(self.SourcePort)),
		netip.AddrPortFrom(ip, uint16(self.StreamPort)), true
}

// controlEndpoint is a playback node's CONTROL_PORT endpoint from its NodeView.
func controlEndpoint(nv contracts.NodeView) (netip.AddrPort, bool) {
	if nv.ControlPort == 0 {
		return netip.AddrPort{}, false
	}
	ip, ok := firstHost(nv.Addrs)
	if !ok {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(ip, uint16(nv.ControlPort)), true
}

// firstHost extracts a usable host IP from a CIDR/host list, preferring a non-
// loopback unicast address, falling back to the first parseable one.
func firstHost(addrs []string) (netip.Addr, bool) {
	var fallback netip.Addr
	haveFallback := false
	for _, a := range addrs {
		ip, ok := parseHost(a)
		if !ok {
			continue
		}
		if !haveFallback {
			fallback, haveFallback = ip, true
		}
		if !ip.IsLoopback() && !ip.IsUnspecified() {
			return ip, true
		}
	}
	return fallback, haveFallback
}

// parseHost accepts "ip/prefix" (CIDR) or a bare IP and returns the address.
func parseHost(s string) (netip.Addr, bool) {
	if pfx, err := netip.ParsePrefix(s); err == nil {
		return pfx.Addr(), true
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip, true
	}
	return netip.Addr{}, false
}

// volPct converts a 0.0–1.0 software gain to a 0–100 percent for SETVOL.
func volPct(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 100
	}
	return uint8(math.Round(v * 100))
}
