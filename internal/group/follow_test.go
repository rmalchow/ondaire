package group

import (
	"testing"

	"ondaire/internal/contracts"
	"ondaire/internal/id"
)

// follow sets the node's PLAYER target (D49+). Any id is allowed — including self
// (play own group) — and there is no rejection of dead/unknown/follower targets.

func TestFollowSetsTarget(t *testing.T) {
	self, target := idN(1), idN(2)
	r := newRig(self, 0, false)
	snap := soloSnap(self)
	snap.Nodes = append(snap.Nodes, node(target, target, true))
	r.cl.setSnap(snap)

	if err := r.e.Follow(target); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	got, ok := r.cl.lastFollowing()
	if !ok || got != target {
		t.Fatalf("SetFollowing = %v,%v want %v", got, ok, target)
	}
}

// Following self is now allowed: it means "play my own group".
func TestFollowSelfAllowed(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Follow(self); err != nil {
		t.Fatalf("Follow(self) should be allowed (play own group): %v", err)
	}
	if got, ok := r.cl.lastFollowing(); !ok || got != self {
		t.Fatalf("SetFollowing = %v,%v want self", got, ok)
	}
}

func TestFollowPersistsFollowing(t *testing.T) {
	self, target := idN(1), idN(2)
	r := newRig(self, 0, false)
	snap := soloSnap(self)
	snap.Nodes = append(snap.Nodes, node(target, target, true))
	r.cl.setSnap(snap)

	if err := r.e.Follow(target); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	// D45: the engine writes BOTH the replicated cluster record AND persists it.
	if got, ok := r.cl.lastFollowing(); !ok || got != target {
		t.Fatalf("cluster SetFollowing = %v,%v want %v", got, ok, target)
	}
	if got, ok := r.lastPersisted(); !ok || got != target {
		t.Fatalf("PersistFollowing = %v,%v want %v", got, ok, target)
	}

	// Unfollow persists the clear (Zero = idle) too.
	if err := r.e.Unfollow(); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	if got, ok := r.lastPersisted(); !ok || !got.IsZero() {
		t.Fatalf("PersistFollowing after Unfollow = %v,%v want Zero", got, ok)
	}
}

// Re-pointing the player from one target to another is allowed.
func TestFollowRepoint(t *testing.T) {
	self, a, b := idN(1), idN(2), idN(3)
	r := newRig(self, 0, false)
	snap := contracts.Snapshot{
		Nodes: []contracts.NodeView{
			node(self, a, true),
			node(a, a, true),
			node(b, b, true),
		},
		Groups: []contracts.GroupView{
			{ID: a, Master: a, Members: []id.ID{a, self}},
			{ID: b, Master: b, Members: []id.ID{b}},
		},
	}
	r.cl.setSnap(snap)
	if err := r.e.Follow(b); err != nil {
		t.Fatalf("Follow(b): %v", err)
	}
	if got, _ := r.cl.lastFollowing(); got != b {
		t.Fatalf("SetFollowing = %v, want %v", got, b)
	}
}

// Unfollow sets the player idle (Following = Zero).
func TestUnfollow(t *testing.T) {
	self := idN(1)
	r := newRig(self, 0, false)
	r.cl.setSnap(soloSnap(self))
	if err := r.e.Unfollow(); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	got, ok := r.cl.lastFollowing()
	if !ok || got != id.Zero {
		t.Fatalf("SetFollowing = %v,%v want Zero", got, ok)
	}
}
