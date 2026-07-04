package playback

import (
	"net/netip"
	"testing"

	"ondaire/internal/contracts"
	"ondaire/internal/id"
	"ondaire/internal/stream"
)

type fakeStore struct {
	self id.ID
	snap contracts.Snapshot
	ch   chan struct{}
}

func (s *fakeStore) Self() id.ID                  { return s.self }
func (s *fakeStore) Snapshot() contracts.Snapshot { return s.snap }
func (s *fakeStore) Subscribe() <-chan struct{}   { return s.ch }

func newDriver(self id.ID) (*Driver, *capturingWriter, *fakeStore) {
	w := &capturingWriter{}
	store := &fakeStore{self: self, ch: make(chan struct{}, 1)}
	d := NewDriver(DriverConfig{Store: store, W: w})
	return d, w, store
}

// masterSnap builds a snapshot: a master `self` (with ports + addr) mastering a
// group, plus a playback node member with the given playback state.
func masterSnap(self, pb id.ID, state string) contracts.Snapshot {
	return contracts.Snapshot{
		Nodes: []contracts.NodeView{
			{ID: self, Addrs: []string{"10.0.0.1/24"}, SourcePort: 9200, StreamPort: 9090},
			{ID: pb, PlaybackNode: true, ControlPort: 9300, Addrs: []string{"10.0.0.7/32"}, Volume: 0.5, OutputDelayMs: 20, Following: self},
		},
		Groups: []contracts.GroupView{{
			ID: self, Master: self, Members: []id.ID{self, pb},
			Playback: contracts.Playback{State: state},
			Settings: contracts.GroupSettings{Codec: "opus", Transport: "udp", BufferMs: 150},
		}},
	}
}

func TestDriverPollsIdleUnassignedPlaybackNode(t *testing.T) {
	self, pb := id.New(), id.New()
	d, w, store := newDriver(self)
	// pb is discovered but UNASSIGNED (following Zero) and in no one's group.
	store.snap = contracts.Snapshot{
		Nodes: []contracts.NodeView{
			{ID: self, Addrs: []string{"10.0.0.1/24"}, SourcePort: 9200, StreamPort: 9090},
			{ID: pb, PlaybackNode: true, ControlPort: 9300, Addrs: []string{"10.0.0.7/32"}},
		},
		Groups: []contracts.GroupView{{ID: self, Master: self, Members: []id.ID{}}},
	}
	d.reconcile(store.snap)
	hs := packetsTo(w, netip.MustParseAddrPort("10.0.0.7:9300"))
	// Liveness poll keeps an idle node alive + re-addable, but it is NOT driven.
	if !hasType(hs, stream.TypeStatusReq) {
		t.Fatal("expected a STATUS_REQ liveness poll to an idle unassigned playback node")
	}
	if hasType(hs, stream.TypeAttach) || hasType(hs, stream.TypeSetVol) {
		t.Fatal("must not ATTACH/SETVOL an unassigned node")
	}
}

func packetsTo(w *capturingWriter, dst netip.AddrPort) []stream.Header {
	w.mu.Lock()
	defer w.mu.Unlock()
	var hs []stream.Header
	for _, wr := range w.writes {
		if wr.dst != dst {
			continue
		}
		if h, _, err := stream.DecodeFrame(wr.pkt); err == nil {
			hs = append(hs, h)
		}
	}
	return hs
}

func hasType(hs []stream.Header, typ byte) bool {
	for _, h := range hs {
		if h.Type == typ {
			return true
		}
	}
	return false
}

func TestDriverAttachesPlayingAssignedNode(t *testing.T) {
	self, pb := id.New(), id.New()
	d, w, store := newDriver(self)
	store.snap = masterSnap(self, pb, "playing")
	d.reconcile(store.snap)

	ctrl := netip.MustParseAddrPort("10.0.0.7:9300")
	hs := packetsTo(w, ctrl)
	// ATTACH + SETVOL + SETDELAY. A non-gossiping playback node keeps no node.json,
	// so the master's record is the only source of its output delay — the driver
	// MUST push SETDELAY (the node dedups). It carries the record's OutputDelayMs.
	if !hasType(hs, stream.TypeAttach) || !hasType(hs, stream.TypeSetVol) {
		t.Fatalf("expected ATTACH+SETVOL, got %d packets", len(hs))
	}
	if !hasType(hs, stream.TypeSetDelay) {
		t.Fatal("driver must push SETDELAY to a playback node (its delay is master-owned)")
	}
	// The pushed delay must equal the node record's OutputDelayMs (20 ms).
	var sawDelay bool
	for _, wr := range w.writes {
		h, payload, err := stream.DecodeFrame(wr.pkt)
		if err != nil || h.Type != stream.TypeSetDelay {
			continue
		}
		sd, _ := stream.DecodeSetDelay(payload)
		if sd.DelayMs != 20 {
			t.Fatalf("SETDELAY DelayMs = %d, want 20", sd.DelayMs)
		}
		sawDelay = true
	}
	if !sawDelay {
		t.Fatal("no SETDELAY packet decoded")
	}

	// Verify the ATTACH carries the master's own endpoints + the group settings.
	w.mu.Lock()
	defer w.mu.Unlock()
	var found bool
	for _, wr := range w.writes {
		h, payload, err := stream.DecodeFrame(wr.pkt)
		if err != nil || h.Type != stream.TypeAttach {
			continue
		}
		a, _ := stream.DecodeAttach(payload)
		if a.Source != netip.MustParseAddrPort("10.0.0.1:9200") ||
			a.Clock != netip.MustParseAddrPort("10.0.0.1:9090") ||
			a.Codec != stream.CodecOpus || a.BufferMs != 150 {
			t.Fatalf("attach payload: %+v", a)
		}
		found = true
	}
	if !found {
		t.Fatal("no ATTACH packet found")
	}
}

func TestDriverDetachesIdleGroup(t *testing.T) {
	self, pb := id.New(), id.New()
	d, w, store := newDriver(self)
	store.snap = masterSnap(self, pb, "idle")
	d.reconcile(store.snap)

	ctrl := netip.MustParseAddrPort("10.0.0.7:9300")
	hs := packetsTo(w, ctrl)
	if hasType(hs, stream.TypeAttach) {
		t.Fatal("idle group must not ATTACH")
	}
	if !hasType(hs, stream.TypeDetach) {
		t.Fatal("idle group should DETACH the assigned node")
	}
	// Config is the node's state, not a property of playback: volume + channel are
	// asserted even while the group is idle, so a volume change takes effect at once
	// (not only once the group starts playing).
	if !hasType(hs, stream.TypeSetVol) {
		t.Fatal("idle group should still push SETVOL (config is not gated by playing)")
	}
}

func TestDriverDetachesUnassignedNode(t *testing.T) {
	self, pb := id.New(), id.New()
	d, w, _ := newDriver(self)
	// First: playing + assigned → tracked.
	d.reconcile(masterSnap(self, pb, "playing"))
	if len(d.active) != 1 {
		t.Fatalf("expected 1 active node, got %d", len(d.active))
	}
	// Then the node leaves the group (members no longer include it).
	gone := masterSnap(self, pb, "playing")
	gone.Groups[0].Members = []id.ID{self}
	w.writes = nil
	d.reconcile(gone)

	if len(d.active) != 0 {
		t.Fatalf("unassigned node still tracked: %d", len(d.active))
	}
	ctrl := netip.MustParseAddrPort("10.0.0.7:9300")
	if !hasType(packetsTo(w, ctrl), stream.TypeDetach) {
		t.Fatal("unassigned node should be DETACHed")
	}
}

func TestDriverIgnoresGroupsItDoesNotMaster(t *testing.T) {
	self, other, pb := id.New(), id.New(), id.New()
	d, w, store := newDriver(self)
	// The playback node is in OTHER's group, not ours.
	store.snap = contracts.Snapshot{
		Nodes: []contracts.NodeView{
			{ID: self, Addrs: []string{"10.0.0.1/24"}, SourcePort: 9200, StreamPort: 9090},
			{ID: other, Addrs: []string{"10.0.0.2/24"}, SourcePort: 9200, StreamPort: 9090},
			{ID: pb, PlaybackNode: true, ControlPort: 9300, Addrs: []string{"10.0.0.7/32"}, Following: other},
		},
		Groups: []contracts.GroupView{{
			ID: other, Master: other, Members: []id.ID{other, pb},
			Playback: contracts.Playback{State: "playing"},
			Settings: contracts.GroupSettings{Codec: "opus", Transport: "udp", BufferMs: 150},
		}},
	}
	d.reconcile(store.snap)
	// We may liveness-poll ANY known playback node (STATUS_REQ), but must not DRIVE
	// one in a group we don't master (no ATTACH/SETVOL/SETDELAY/DETACH).
	ctrl := netip.MustParseAddrPort("10.0.0.7:9300")
	hs := packetsTo(w, ctrl)
	for _, h := range hs {
		if h.Type != stream.TypeStatusReq {
			t.Fatalf("must not drive a node in a group we don't master; sent type 0x%x", h.Type)
		}
	}
	if len(d.active) != 0 {
		t.Fatal("must not track a node in another master's group")
	}
}

// twoRoomSnap: master `self` plays a group with two playback rooms at distinct
// control endpoints (…7 and …8 on 9300), so the driver can equalize between them.
func twoRoomSnap(self, slow, fast id.ID) contracts.Snapshot {
	return contracts.Snapshot{
		Nodes: []contracts.NodeView{
			{ID: self, Addrs: []string{"10.0.0.1/24"}, SourcePort: 9200, StreamPort: 9090},
			{ID: slow, PlaybackNode: true, ControlPort: 9300, Addrs: []string{"10.0.0.7/32"}, Following: self},
			{ID: fast, PlaybackNode: true, ControlPort: 9300, Addrs: []string{"10.0.0.8/32"}, Following: self},
		},
		Groups: []contracts.GroupView{{
			ID: self, Master: self, Members: []id.ID{self, slow, fast},
			Playback: contracts.Playback{State: "playing"},
			Settings: contracts.GroupSettings{Codec: "opus", Transport: "udp", BufferMs: 300},
		}},
	}
}

// lastEqualizeMs returns the DelayMs of the most recent SETEQ packet sent to dst, or
// -1 if none was sent.
func lastEqualizeMs(w *capturingWriter, dst netip.AddrPort) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	got := -1
	for _, wr := range w.writes {
		if wr.dst != dst {
			continue
		}
		h, payload, err := stream.DecodeFrame(wr.pkt)
		if err != nil || h.Type != stream.TypeSetEq {
			continue
		}
		if e, err := stream.DecodeSetEqualize(payload); err == nil {
			got = int(e.DelayMs)
		}
	}
	return got
}

// D65: with both rooms calibrated, the slower room (larger device buffer) sets the
// reference; the faster room is delayed by the difference so speaker_times align.
func TestDriverEqualizesCrossRoomDelay(t *testing.T) {
	self, slow, fast := id.New(), id.New(), id.New()
	d, w, store := newDriver(self)
	store.snap = twoRoomSnap(self, slow, fast)
	d.delays = func() map[id.ID]RoomDelay {
		return map[id.ID]RoomDelay{
			slow: {SetpointNs: 250_000_000, Calibrated: true, Playing: true},
			fast: {SetpointNs: 180_000_000, Calibrated: true, Playing: true},
		}
	}
	d.reconcile(store.snap)

	if got := lastEqualizeMs(w, netip.MustParseAddrPort("10.0.0.8:9300")); got != 70 {
		t.Fatalf("fast room equalize = %d ms, want 70 (250−180)", got)
	}
	if got := lastEqualizeMs(w, netip.MustParseAddrPort("10.0.0.7:9300")); got != 0 {
		t.Fatalf("slow (reference) room equalize = %d ms, want 0", got)
	}
}

// D65: until EVERY device-bearing room has a settled setpoint, the max is not final,
// so the driver pushes no equalization (acting early would re-anchor again).
func TestDriverWaitsForAllRoomsCalibrated(t *testing.T) {
	self, slow, fast := id.New(), id.New(), id.New()
	d, w, store := newDriver(self)
	store.snap = twoRoomSnap(self, slow, fast)
	d.delays = func() map[id.ID]RoomDelay {
		return map[id.ID]RoomDelay{
			slow: {SetpointNs: 250_000_000, Calibrated: true, Playing: true},
			fast: {SetpointNs: 180_000_000, Calibrated: false, Playing: true}, // still settling
		}
	}
	d.reconcile(store.snap)

	if got := lastEqualizeMs(w, netip.MustParseAddrPort("10.0.0.8:9300")); got != -1 {
		t.Fatalf("equalize pushed (%d ms) before all rooms calibrated", got)
	}
}

func TestVolPct(t *testing.T) {
	cases := map[float64]uint8{-0.1: 0, 0: 0, 0.5: 50, 0.504: 50, 0.999: 100, 1: 100, 2: 100, 0.73: 73}
	for in, want := range cases {
		if got := volPct(in); got != want {
			t.Errorf("volPct(%v) = %d, want %d", in, got, want)
		}
	}
}
