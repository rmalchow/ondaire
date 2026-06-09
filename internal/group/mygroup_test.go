package group

import (
	"testing"

	"ensemble/internal/contracts"
)

// A node always masters its own group; following itself targets its own group.
func TestMyGroupMastersOwnGroupSelfTarget(t *testing.T) {
	self := idN(1)
	mv := myGroup(soloSnap(self), self)
	if !mv.found {
		t.Fatal("want found")
	}
	if mv.group.Master != self {
		t.Fatalf("self must master its own group: group.Master=%v want %v", mv.group.Master, self)
	}
	if !mv.hasTarget || mv.target.Master != self {
		t.Fatalf("Following==self ⇒ player target is own group: hasTarget=%v target.Master=%v", mv.hasTarget, mv.target.Master)
	}
}

func TestMyGroupMaster(t *testing.T) {
	self, f := idN(1), idN(2)
	mv := myGroup(masterSnap(self, defaultSettings(), f), self)
	if mv.group.Master != self {
		t.Fatalf("master = %v, want self", mv.group.Master)
	}
	if len(mv.group.Members) == 0 { // own group has players (self self-follows + f)
		t.Fatal("own group should have players")
	}
}

// self's player targets another master's group, while self still masters its OWN.
func TestMyGroupPlayerTargetIsFollowed(t *testing.T) {
	master, self := idN(1), idN(2)
	mv := myGroup(masterSnap(master, defaultSettings(), self), self)
	if !mv.found || mv.group.Master != self {
		t.Fatalf("self always masters its own group: master=%v", mv.group.Master)
	}
	if !mv.hasTarget || mv.target.Master != master {
		t.Fatalf("player target should be the followed master's group: hasTarget=%v target.Master=%v", mv.hasTarget, mv.target.Master)
	}
}

// Following a dead/unknown node ⇒ no player target (idle); no "stale" concept.
func TestMyGroupIdleWhenTargetDead(t *testing.T) {
	self, deadTarget := idN(1), idN(9)
	n := node(self, deadTarget, true)
	g := contracts.GroupView{ID: self, Master: self} // only self's own (empty) group is derived
	snap := contracts.Snapshot{Nodes: []contracts.NodeView{n}, Groups: []contracts.GroupView{g}}
	mv := myGroup(snap, self)
	if !mv.found || mv.group.Master != self {
		t.Fatal("self still masters its own group")
	}
	if mv.hasTarget {
		t.Fatal("dead/unknown target ⇒ no player target (idle)")
	}
}

func TestMyGroupNotYetDerived(t *testing.T) {
	self := idN(1)
	mv := myGroup(contracts.Snapshot{}, self)
	if mv.found {
		t.Fatal("want not found")
	}
}
