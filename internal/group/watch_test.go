package group

import (
	"testing"

	"ondaire/internal/contracts"
)

// The engine is a pure PRODUCER now: reconcile drives no subscriber/sink/clock.
// A cluster change is absorbed without panic and without any player-side wiring;
// only producer effects (composition logging, heartbeat, renegotiation) run.
func TestReconcileAbsorbsClusterChange(t *testing.T) {
	self, a, b := idN(1), idN(2), idN(3)
	r := newRig(self, 0, false)

	// self follows a; the group has an active session.
	r.cl.setSnap(withPlaying(masterSnap(a, defaultSettings(), self)))
	r.e.reconcile() // must not panic; no session here → no playback write
	if _, ok := r.cl.lastPlayback(); ok {
		t.Fatal("non-sourcing node must not write playback on reconcile")
	}

	// Master changes to b: still a no-op for a non-sourcing node.
	r.cl.setSnap(withPlaying(masterSnap(b, defaultSettings(), self)))
	r.e.reconcile() // must not panic
}

func TestReconcileSkipsBeforeSelfDerived(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(contracts.Snapshot{}) // self not present
	r.e.reconcile()                    // must not panic
	if _, ok := r.cl.lastPlayback(); ok {
		t.Fatal("no producer effect expected before self derived")
	}
}

// A player whose target master is dead/unknown idles, with no follow-reset
// (self-heal is obsolete under the crosswise model). The engine drives no
// player-side wiring; reconcile must simply absorb it.
func TestPlayerIdleWhenTargetDead(t *testing.T) {
	self, dead := idN(1), idN(9)
	r := newRig(self, 0, false)
	// self's player targets `dead`, which is not a derived master → idle.
	n := node(self, dead, true)
	g := contracts.GroupView{ID: self, Master: self, Members: nil} // self's own (empty) group
	r.cl.setSnap(contracts.Snapshot{Nodes: []contracts.NodeView{n}, Groups: []contracts.GroupView{g}})

	r.e.reconcile() // must not panic
	// following is NOT reset (no self-heal): the player just idles until the target returns.
	if _, ok := r.cl.lastFollowing(); ok {
		t.Fatal("dead target must not trigger a follow reset")
	}
}
