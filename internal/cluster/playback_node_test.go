package cluster

import (
	"net/netip"
	"path/filepath"
	"testing"

	"ondaire/internal/discovery"
	"ondaire/internal/id"
)

func pbPeer(pid id.ID, control int, codecs ...string) discovery.Peer {
	return discovery.Peer{
		ID:          pid,
		Addr:        netip.MustParseAddr("10.0.0.7"),
		Playback:    true,
		ControlPort: control,
		Caps:        discovery.Caps{Codecs: codecs, MaxRate: 48000},
	}
}

func newMaster(t *testing.T) *Cluster {
	t.Helper()
	c, err := New(Config{Self: id.New(), Name: "m", GossipPort: 7946})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func recOf(c *Cluster, nid id.ID) *NodeRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.doc.Nodes[nid]
}

func TestUpsertPlaybackNodeRepresentsButDoesNotGroup(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus", "pcm"))

	r := recOf(c, pid)
	if r == nil || !r.PlaybackNode || r.ControlPort != 9300 || r.Version != 1 {
		t.Fatalf("proxied record wrong: %+v", r)
	}
	if len(r.Addrs) != 1 || r.Addrs[0] != "10.0.0.7/32" {
		t.Fatalf("addrs = %v", r.Addrs)
	}
	if len(r.Caps.Codecs) != 2 || r.Caps.Codecs[0] != "opus" || !r.Caps.Playback {
		t.Fatalf("caps = %+v", r.Caps)
	}

	snap := c.Snapshot()
	// Appears in Nodes...
	inNodes := false
	for _, nv := range snap.Nodes {
		if nv.ID == pid {
			inNodes = true
			if !nv.PlaybackNode || nv.ControlPort != 9300 {
				t.Fatalf("nodeview: playback=%v control=%d", nv.PlaybackNode, nv.ControlPort)
			}
		}
	}
	if !inNodes {
		t.Fatal("playback node missing from Snapshot.Nodes")
	}
	// ...but is NOT a member of any derived group (slice-A scope).
	for _, g := range snap.Groups {
		for _, m := range g.Members {
			if m == pid {
				t.Fatalf("playback node must not appear in group %v members yet", g.ID)
			}
		}
	}
}

func TestUpsertPlaybackNodeContentGated(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	v1 := recOf(c, pid).Version

	// Identical re-ingest: no version bump (content-gated).
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	if got := recOf(c, pid).Version; got != v1 {
		t.Fatalf("identical re-ingest bumped version %d → %d", v1, got)
	}

	// Changed control port: version bumps, port updates.
	c.UpsertPlaybackNode(pbPeer(pid, 9400, "opus"))
	r := recOf(c, pid)
	if r.Version != v1+1 || r.ControlPort != 9400 {
		t.Fatalf("changed re-ingest: version=%d control=%d", r.Version, r.ControlPort)
	}
}

func TestUpsertPlaybackNodePreservesAssignment(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	master := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))

	// Simulate an operator assignment (Slice-C will expose a setter for this).
	c.mu.Lock()
	c.doc.Nodes[pid].Following = master
	c.mu.Unlock()

	// Re-ingest (identical advert) must preserve the assignment.
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	if got := recOf(c, pid).Following; got != master {
		t.Fatalf("assignment not preserved: Following = %v, want %v", got, master)
	}
	// Even a content-changing re-ingest preserves it.
	c.UpsertPlaybackNode(pbPeer(pid, 9400, "opus"))
	if got := recOf(c, pid).Following; got != master {
		t.Fatalf("assignment lost on content change: %v", got)
	}
}

// assignPlayback simulates an operator assignment (the Slice-C setter), then
// re-ingests so the proxied record carries Following.
func assignPlayback(c *Cluster, pid, master id.ID) {
	c.mu.Lock()
	c.doc.Nodes[pid].Following = master
	c.mu.Unlock()
}

func groupContains(c *Cluster, master, member id.ID) bool {
	for _, g := range c.Snapshot().Groups {
		if g.Master == master {
			for _, m := range g.Members {
				if m == member {
					return true
				}
			}
		}
	}
	return false
}

func TestAssignedPlaybackNodeJoinsGroup(t *testing.T) {
	c := newMaster(t) // self is a live solo master
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))

	// Unassigned: present in Nodes, but in no group (not even its own).
	if groupContains(c, pid, pid) {
		t.Fatal("unassigned playback node must not form a solo group")
	}
	for _, g := range c.Snapshot().Groups {
		for _, m := range g.Members {
			if m == pid {
				t.Fatalf("unassigned playback node leaked into group %v", g.ID)
			}
		}
	}

	// Assign to self (the live master) → joins self's group.
	assignPlayback(c, pid, c.self)
	if !groupContains(c, c.self, pid) {
		t.Fatal("assigned playback node did not join its master's group")
	}
	// And it is never itself a master.
	if groupContains(c, pid, pid) {
		t.Fatal("playback node must never master its own group")
	}
}

func TestPlaybackNodeStaleAssignmentNotGrouped(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	assignPlayback(c, pid, id.New())   // follow a master that doesn't exist
	if len(c.Snapshot().Groups) != 1 { // only self's solo group
		t.Fatalf("stale-assigned playback node created a phantom group: %d groups", len(c.Snapshot().Groups))
	}
	if groupContains(c, c.self, pid) {
		t.Fatal("playback node must not attach to a non-existent master")
	}
}

func TestStalePlaybackNodeNotAlive(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	// Age its record beyond the TTL.
	c.mu.Lock()
	c.doc.Nodes[pid].UpdatedAt -= playbackLivenessTTLSec + 5
	c.mu.Unlock()
	assignPlayback(c, pid, c.self)
	if groupContains(c, c.self, pid) {
		t.Fatal("a stale (TTL-expired) playback node must not be grouped")
	}
	for _, nv := range c.Snapshot().Nodes {
		if nv.ID == pid && nv.Alive {
			t.Fatal("stale playback node should not be Alive")
		}
	}
}

func TestAssignPlaybackNodeSetterGroupsAndClears(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))

	// Assign to self (a live master) → joins the group.
	if !c.AssignPlaybackNode(pid, c.self) {
		t.Fatal("AssignPlaybackNode should report a change")
	}
	if !groupContains(c, c.self, pid) {
		t.Fatal("assigned node not in master's group")
	}
	// Idempotent: same assignment is a no-op.
	if c.AssignPlaybackNode(pid, c.self) {
		t.Fatal("re-assigning the same target should be a no-op")
	}
	// Clear (Zero) → leaves the group.
	if !c.AssignPlaybackNode(pid, id.Zero) {
		t.Fatal("unassign should report a change")
	}
	if groupContains(c, c.self, pid) {
		t.Fatal("cleared node still in group")
	}
}

func TestAssignPlaybackNodeRejectsNonPlaybackAndUnknown(t *testing.T) {
	c := newMaster(t)
	// Unknown node.
	if c.AssignPlaybackNode(id.New(), c.self) {
		t.Fatal("assigning an unknown node must be a no-op")
	}
	// A gossiping node (self) owns its own Following — the setter must refuse.
	if c.AssignPlaybackNode(c.self, id.New()) {
		t.Fatal("AssignPlaybackNode must refuse a non-playback (gossiping) node")
	}
}

func TestUpsertPlaybackNodeUsesAdvertisedName(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	p := pbPeer(pid, 9300, "opus")
	p.Name = "kitchen"
	c.UpsertPlaybackNode(p)
	if got := recOf(c, pid).Name; got != "kitchen" {
		t.Fatalf("advertised name not stored: %q", got)
	}
}

func TestPatchPlaybackNode(t *testing.T) {
	c := newMaster(t)
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))

	name, vol, delay, master := "den", 0.5, 20, c.self
	if !c.PatchPlaybackNode(pid, &name, &vol, &delay, &master, nil) {
		t.Fatal("patch should succeed on a playback node")
	}
	r := recOf(c, pid)
	if r.Name != "den" || r.Volume != 0.5 || r.OutputDelayMs != 20 || r.Following != master {
		t.Fatalf("patch not applied: %+v", r)
	}
	// A master-set name survives re-discovery (UpsertPlaybackNode preserves it).
	c.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	if recOf(c, pid).Name != "den" {
		t.Fatalf("master-set name lost on re-discovery: %q", recOf(c, pid).Name)
	}
	// Refuses a gossiping (non-playback) node.
	if c.PatchPlaybackNode(c.self, &name, nil, nil, nil, nil) {
		t.Fatal("PatchPlaybackNode must refuse a non-playback node")
	}
}

func TestUpsertPlaybackNodeRejectsSelfAndZeroControl(t *testing.T) {
	c := newMaster(t)
	c.UpsertPlaybackNode(pbPeer(c.self, 9300, "opus")) // self
	if recOf(c, c.self).PlaybackNode {
		t.Fatal("self record must not be marked a playback node")
	}
	pid := id.New()
	c.UpsertPlaybackNode(pbPeer(pid, 0, "opus")) // no control port
	if recOf(c, pid) != nil {
		t.Fatal("peer with no control port must be rejected")
	}
}

// TestPlaybackAssignmentSurvivesMasterRestart is the regression guard for the bug
// where a master restart silently dropped every playback node back to solo: the
// proxy record is rebuilt from mDNS (which carries no assignment), so the operator
// assignment must be persisted and restored on re-discovery (D59).
func TestPlaybackAssignmentSurvivesMasterRestart(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "cluster.json")
	self := id.New()
	pid := id.New()
	master := self // assign the playback node to the master's own group

	// First master lifetime: discover the node, assign it, persist on Close.
	c1, err := New(Config{Self: self, Name: "m", GossipPort: 7946, StatePath: statePath})
	if err != nil {
		t.Fatalf("New c1: %v", err)
	}
	c1.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	if !c1.AssignPlaybackNode(pid, master) {
		t.Fatal("AssignPlaybackNode returned false")
	}
	if got := recOf(c1, pid).Following; got != master {
		t.Fatalf("assignment not applied: Following=%v want %v", got, master)
	}
	c1.Close() // final saveState writes playbackAssignments to statePath

	// Second master lifetime (simulated restart): same statePath, fresh doc.
	c2, err := New(Config{Self: self, Name: "m", GossipPort: 7946, StatePath: statePath})
	if err != nil {
		t.Fatalf("New c2: %v", err)
	}
	defer c2.Close()
	if recOf(c2, pid) != nil {
		t.Fatal("proxy must not exist before re-discovery")
	}
	// mDNS re-discovers the node — Following must be restored, not dropped to solo.
	c2.UpsertPlaybackNode(pbPeer(pid, 9300, "opus"))
	if got := recOf(c2, pid).Following; got != master {
		t.Fatalf("assignment NOT restored after restart: Following=%v want %v (regression: playback node went solo)", got, master)
	}

	// Clearing the assignment removes it from persistence too.
	if !c2.AssignPlaybackNode(pid, id.Zero) {
		t.Fatal("clear assignment returned false")
	}
	c2.mu.Lock()
	_, stillThere := c2.pbAssign[pid]
	c2.mu.Unlock()
	if stillThere {
		t.Fatal("cleared assignment still present in pbAssign")
	}
}
