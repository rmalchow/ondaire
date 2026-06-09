package group

import (
	"net/netip"
	"testing"

	"ensemble/internal/contracts"
)

func TestRepointSubscriberOnMasterChange(t *testing.T) {
	self, a, b := idN(1), idN(2), idN(3)
	r := newRig(self, 0, false)
	r.cl.dialResults[a] = []netip.Addr{netip.AddrFrom4([4]byte{127, 0, 0, 2})}
	r.cl.dialResults[b] = []netip.Addr{netip.AddrFrom4([4]byte{127, 0, 0, 3})}

	// self follows a; the group has an active session (subscribe is gated on it).
	r.cl.setSnap(withPlaying(masterSnap(a, defaultSettings(), self)))
	r.e.reconcile()

	subs := r.sub.snapshotSubs()
	if len(subs) != 1 {
		t.Fatalf("subs after first reconcile = %d, want 1", len(subs))
	}
	if subs[0].addr.Addr() != netip.AddrFrom4([4]byte{127, 0, 0, 2}) {
		t.Fatalf("sub addr = %v", subs[0].addr)
	}

	// Idempotent: unchanged master → no new Subscribe.
	r.e.reconcile()
	if got := len(r.sub.snapshotSubs()); got != 1 {
		t.Fatalf("subs after repeat reconcile = %d, want 1 (idempotent)", got)
	}

	// Master changes to b.
	r.cl.setSnap(withPlaying(masterSnap(b, defaultSettings(), self)))
	r.e.reconcile()
	subs = r.sub.snapshotSubs()
	if len(subs) != 2 {
		t.Fatalf("subs after master change = %d, want 2", len(subs))
	}
	if subs[1].addr.Addr() != netip.AddrFrom4([4]byte{127, 0, 0, 3}) {
		t.Fatalf("new sub addr = %v", subs[1].addr)
	}
	// Clock follower + sink re-pointed too.
	if len(r.cc.snapshot()) != 2 {
		t.Fatalf("clockctl calls = %d, want 2", len(r.cc.snapshot()))
	}
	if len(r.snk.snapshotResets()) != 2 {
		t.Fatalf("sink resets = %d, want 2", len(r.snk.snapshotResets()))
	}
}

func TestRepointUsesDialCandidatesAndPorts(t *testing.T) {
	self, master := idN(1), idN(2)
	r := newRig(self, 0, false)
	masterIP := netip.AddrFrom4([4]byte{10, 0, 0, 5})
	r.cl.dialResults[master] = []netip.Addr{masterIP}

	snap := withPlaying(masterSnap(master, defaultSettings(), self))
	// give the master distinct ports.
	for i := range snap.Nodes {
		if snap.Nodes[i].ID == master {
			snap.Nodes[i].SourcePort = 9300
			snap.Nodes[i].StreamPort = 9190
		}
	}
	r.cl.setSnap(snap)
	r.e.reconcile()

	subs := r.sub.snapshotSubs()
	if len(subs) != 1 {
		t.Fatalf("subs = %d", len(subs))
	}
	if subs[0].addr != netip.AddrPortFrom(masterIP, 9300) {
		t.Fatalf("sub addr = %v, want %v:9300", subs[0].addr, masterIP)
	}
	cc := r.cc.snapshot()
	if cc[0].dst != netip.AddrPortFrom(masterIP, 9190) {
		t.Fatalf("clock addr = %v, want %v:9190", cc[0].dst, masterIP)
	}
}

func TestMasterSubscribesToSelfLoopback(t *testing.T) {
	self := idN(1)
	r := newRig(self, 1000, true)
	// self plays its own group (Following == self); Play sources it.
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Play("input:"); err != nil {
		t.Fatalf("Play: %v", err)
	}
	defer r.e.Close()

	subs := r.sub.snapshotSubs()
	if len(subs) != 1 {
		t.Fatalf("subs = %d, want 1", len(subs))
	}
	if !subs[0].addr.Addr().IsLoopback() {
		t.Fatalf("self sub addr = %v, want loopback", subs[0].addr)
	}
}

// (Removed: TestWatchTearsDownSessionOnMasterLoss — under the crosswise model a node
// is always the master of its own group, so it never "loses mastership"; a session
// only ends via Stop/EOF/Close. And TestReconcileHealViaRun — self-heal is obsolete:
// a player whose target is dead simply goes idle, no follow reset.)

func TestReconcileSkipsBeforeSelfDerived(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(contracts.Snapshot{}) // self not present
	r.e.reconcile()                    // must not panic
	if len(r.sub.snapshotSubs()) != 0 {
		t.Fatal("no re-point expected before self derived")
	}
}

// A player whose target master is dead/unknown goes IDLE (Detach), with no
// follow-reset (self-heal is obsolete under the crosswise model).
func TestPlayerIdleWhenTargetDead(t *testing.T) {
	self, dead := idN(1), idN(9)
	r := newRig(self, 0, false)
	// self's player targets `dead`, which is not a derived master → idle.
	n := node(self, dead, true)
	g := contracts.GroupView{ID: self, Master: self, Members: nil} // self's own (empty) group
	r.cl.setSnap(contracts.Snapshot{Nodes: []contracts.NodeView{n}, Groups: []contracts.GroupView{g}})

	r.e.reconcile()
	if got := len(r.sub.snapshotSubs()); got != 0 {
		t.Fatalf("idle player must not subscribe; got %d subs", got)
	}
	// following is NOT reset (no self-heal): the player just idles until the target returns.
	if _, ok := r.cl.lastFollowing(); ok {
		t.Fatal("dead target must not trigger a follow reset")
	}
}
