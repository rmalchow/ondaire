package source

import (
	"net/netip"
	"testing"

	"ensemble/internal/id"
	"ensemble/internal/stream"
)

// A STATUS packet (D55) on the SOURCE_PORT control reader is stored keyed by the
// node id in its payload — NOT by the sender address (it comes from the node's
// CONTROL_PORT, a different addr than its audio source).
func TestHandleStatusIngest(t *testing.T) {
	s := NewServer(Config{Self: id.New()})

	nodeID := id.New()
	st := stream.StatusPayload{
		NodeID: [16]byte(nodeID), Synced: true, Playing: true, Played: 42, Buffered: 5, RatePPMx1000: -1250,
	}
	h := stream.Header{Magic: stream.Magic, Type: stream.TypeStatus}
	pkt := h.AppendFrame(nil, st.AppendTo(nil))

	s.handleControlUDP(pkt, netip.MustParseAddrPort("10.0.0.7:55000"))

	got := s.Statuses()
	ps, ok := got[nodeID]
	if !ok {
		t.Fatal("STATUS not ingested")
	}
	if !ps.Status.Synced || !ps.Status.Playing || ps.Status.Played != 42 || ps.Status.Buffered != 5 {
		t.Fatalf("ingested status wrong: %+v", ps.Status)
	}
	if ps.LastSeen.IsZero() {
		t.Fatal("LastSeen not stamped")
	}

	// A malformed STATUS payload is ignored, not stored.
	bad := h.AppendFrame(nil, []byte{1, 2, 3})
	s.handleControlUDP(bad, netip.MustParseAddrPort("10.0.0.8:55000"))
	if len(s.Statuses()) != 1 {
		t.Fatal("malformed STATUS must be ignored")
	}
}
