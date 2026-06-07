package group

import (
	"testing"
	"time"

	"ensemble/internal/contracts"
	"ensemble/internal/id"
)

// staleMV builds a myView with stale=true (dangling follow).
func staleMV(self id.ID) myView {
	n := node(self, idN(9), true)
	g := contracts.GroupView{ID: id.XOR(self), Master: self, Members: []id.ID{self}}
	snap := contracts.Snapshot{Nodes: []contracts.NodeView{n}, Groups: []contracts.GroupView{g}}
	return myGroup(snap, self)
}

func TestHealNoResetBeforeGrace(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	mv := staleMV(self)

	now := r.now
	r.e.reconcileHeal(mv, now)                    // arms healAt = now+10s
	r.e.reconcileHeal(mv, now.Add(9*time.Second)) // still before
	if r.cl.followCount() != 0 {
		t.Fatal("no reset expected before grace")
	}
}

func TestHealResetsAfterGrace(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	mv := staleMV(self)

	now := r.now
	r.e.reconcileHeal(mv, now)
	r.e.reconcileHeal(mv, now.Add(10*time.Second))
	got, ok := r.cl.lastFollowing()
	if !ok || got != id.Zero {
		t.Fatalf("SetFollowing = %v,%v want Zero", got, ok)
	}
	if !r.e.healAt.IsZero() {
		t.Fatal("healAt should be cleared")
	}
}

// TestHealGraceMeasuredFromFirstStaleObservation locks in D45's verification
// point: the grace window starts when the engine first OBSERVES the dangling
// follow, NOT at process start. A node that boots following a master which is
// still converging (seen as a valid follower for a while) must not insta-clear:
// the clock only starts on the first stale reconcile.
func TestHealGraceMeasuredFromFirstStaleObservation(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	now := r.now

	// For the first 30s the master is present → valid follower, never stale.
	master := idN(2)
	valid := myGroup(masterSnap(master, defaultSettings(), self), self)
	r.e.reconcileHeal(valid, now)
	r.e.reconcileHeal(valid, now.Add(30*time.Second))
	if !r.e.healAt.IsZero() {
		t.Fatal("healAt must stay zero while the follow is valid")
	}

	// Master vanishes from this node's view (slow-gossip drop) → stale appears at
	// t+30s. Grace arms HERE, not at t0.
	stale := staleMV(self)
	r.e.reconcileHeal(stale, now.Add(30*time.Second))
	// 9s after the FIRST stale observation: still within grace, no reset.
	r.e.reconcileHeal(stale, now.Add(39*time.Second))
	if r.cl.followCount() != 0 {
		t.Fatal("reset before grace elapsed from first stale observation")
	}
	// 10s after the first stale observation: reset fires + persists the clear.
	r.e.reconcileHeal(stale, now.Add(40*time.Second))
	if got, ok := r.cl.lastFollowing(); !ok || got != id.Zero {
		t.Fatalf("cluster SetFollowing = %v,%v want Zero", got, ok)
	}
	if got, ok := r.lastPersisted(); !ok || !got.IsZero() {
		t.Fatalf("self-heal PersistFollowing = %v,%v want Zero", got, ok)
	}
}

func TestHealCancelsWhenTargetRecovers(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	staleM := staleMV(self)
	now := r.now
	r.e.reconcileHeal(staleM, now) // arm

	// Target recovered: now a valid follower (not stale).
	master := idN(2)
	mv := myGroup(masterSnap(master, defaultSettings(), self), self)
	r.e.reconcileHeal(mv, now.Add(3*time.Second))
	if !r.e.healAt.IsZero() {
		t.Fatal("healAt should be cleared on recover")
	}

	// Even past the original grace, no reset (timer was cancelled).
	r.e.reconcileHeal(mv, now.Add(20*time.Second))
	if r.cl.followCount() != 0 {
		t.Fatal("no reset expected after cancel")
	}
}
