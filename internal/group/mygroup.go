package group

import (
	"ensemble/internal/contracts"
	"ensemble/internal/id"
)

// myView is this node's resolved position in the pre-derived snapshot. Under the
// crosswise model, mastership and playback are independent (D49+):
//
//   - `group` is the group this node MASTERS (Master == self, always 1:1). It is
//     what this node SOURCES when it plays; its Members are the players that follow
//     this node. Used for Play/Stop/settings/codec-negotiation/heartbeat.
//   - `target` is the group this node's PLAYER plays — group(self.Following). It
//     drives the clock/subscriber/sink. hasTarget is false when Following is Zero or
//     points at a dead/unknown/non-master node (the player is then IDLE).
//
// found == false when self has no NodeView or its own group isn't derived yet
// (transient): callers skip reconcile for that tick.
type myView struct {
	self  contracts.NodeView  // this node's own record
	group contracts.GroupView // the group self MASTERS (Master == self)
	found bool

	target    contracts.GroupView // the group self's PLAYER plays (Master == self.Following)
	hasTarget bool                // Following is a live master with a derived group
}

// myGroup resolves this node's master group (group it sources) and player target
// (group it plays), from the pre-derived snapshot. Pure.
func myGroup(snap contracts.Snapshot, self id.ID) myView {
	var mv myView

	var selfNode contracts.NodeView
	haveNode := false
	for _, n := range snap.Nodes {
		if n.ID == self {
			selfNode = n
			haveNode = true
			break
		}
	}
	if !haveNode {
		return mv // found == false
	}

	// The group I master (Master == self) — always derived for an alive node.
	var ownGroup contracts.GroupView
	haveOwn := false
	for _, grp := range snap.Groups {
		if grp.Master == self {
			ownGroup = grp
			haveOwn = true
			break
		}
	}
	if !haveOwn {
		return mv // found == false (self not derived yet)
	}

	mv.self = selfNode
	mv.group = ownGroup
	mv.found = true

	// The group my player plays: group(self.Following), if Following is a live
	// master (a derived group exists for it). Zero / dead / unknown ⇒ idle.
	if t := selfNode.Following; !t.IsZero() {
		for _, grp := range snap.Groups {
			if grp.Master == t {
				mv.target = grp
				mv.hasTarget = true
				break
			}
		}
	}
	return mv
}
