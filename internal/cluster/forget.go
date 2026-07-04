package cluster

import (
	"fmt"

	"ondaire/internal/id"
)

// ForgetNode deletes an OFFLINE node from the cluster. It writes a tombstone (so
// the record is not re-gossiped back by a peer that still holds it), drops the
// node + its group's playback record, scrubs this node's own references to it
// (Following / Spotify endpoint players), and broadcasts both the tombstone and
// the scrubbed own record. Other nodes scrub their own references when they merge
// the tombstone, so a deleted id is purged everywhere it was mentioned.
//
// Refuses self and any node that is currently alive (deleting a live node is
// pointless — it just re-announces; an alive peer also clears the tombstone). A
// node that is already gone still (re)writes a tombstone so the deletion sticks.
func (c *Cluster) ForgetNode(nid id.ID) error {
	if nid == c.self {
		return fmt.Errorf("cannot delete self")
	}
	alive, _ := c.live.snapshot()
	if alive[nid] {
		return fmt.Errorf("node is online")
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("cluster closed")
	}
	now := c.clock().Unix()
	// A freshly-seen proxied playback node (D60) is also "online" by mDNS recency.
	if r := c.doc.Nodes[nid]; r != nil && r.PlaybackNode && now-r.UpdatedAt < playbackLivenessTTLSec {
		c.mu.Unlock()
		return fmt.Errorf("node is online")
	}
	var killed uint64
	if r := c.doc.Nodes[nid]; r != nil {
		killed = r.Version
	}
	tomb := &TombstoneRecord{KilledVersion: killed, UpdatedAt: now}
	c.doc.mergeTombstone(nid, tomb)
	ownSnap := c.scrubOwnRefsLocked(nid)
	delete(c.pbAssign, nid)  // D59: drop any persisted playback assignment for the forgotten node
	delete(c.pbChannel, nid) // and its persisted channel mode
	tombSnap := *tomb
	c.mu.Unlock()

	c.markDirty()
	c.enqueueBroadcast(kindTombstone, nid, delta{Group: nid, Tombstone: &tombSnap})
	if ownSnap != nil {
		c.broadcastOwn(ownSnap)
	}
	c.notify()
	c.log.Info("node forgotten", "node", nid.String(), "killedVersion", killed)
	return nil
}

// scrubOwnRefsLocked removes references to a deleted node from THIS node's own
// record: clears Following when it targets the dead node (→ solo) and drops the
// dead id from every Spotify endpoint's player list. Bumps the record version and
// returns a clone to broadcast when anything changed, else nil. Caller holds mu.
func (c *Cluster) scrubOwnRefsLocked(dead id.ID) *NodeRecord {
	r := c.own()
	if r == nil {
		return nil
	}
	changed := false
	if r.Following == dead {
		r.Following = id.Zero
		changed = true
	}
	for i := range r.SpotifyEndpoints {
		players := r.SpotifyEndpoints[i].Players
		kept := players[:0:0]
		for _, p := range players {
			if p == dead {
				changed = true
				continue
			}
			kept = append(kept, p)
		}
		r.SpotifyEndpoints[i].Players = kept
	}
	if !changed {
		return nil
	}
	r.Version++
	r.UpdatedAt = c.clock().Unix()
	return cloneNode(r)
}

// scrubOwnAgainstTombstonesLocked scrubs this node's own references against EVERY
// known tombstone — used when a tombstone arrives from a peer, so each node purges
// its own config without the deleter having to (and can't) reach into it. Returns
// a clone to broadcast when changed, else nil. Caller holds mu.
func (c *Cluster) scrubOwnAgainstTombstonesLocked() *NodeRecord {
	if len(c.doc.Tombstones) == 0 {
		return nil
	}
	r := c.own()
	if r == nil {
		return nil
	}
	changed := false
	if !r.Following.IsZero() && c.doc.Tombstones[r.Following] != nil {
		r.Following = id.Zero
		changed = true
	}
	for i := range r.SpotifyEndpoints {
		players := r.SpotifyEndpoints[i].Players
		kept := players[:0:0]
		for _, p := range players {
			if c.doc.Tombstones[p] != nil {
				changed = true
				continue
			}
			kept = append(kept, p)
		}
		r.SpotifyEndpoints[i].Players = kept
	}
	if !changed {
		return nil
	}
	r.Version++
	r.UpdatedAt = c.clock().Unix()
	return cloneNode(r)
}
