package group

import (
	"context"
	"net/netip"
	"time"

	"ensemble/internal/id"
	"ensemble/internal/playback"
	"ensemble/internal/stream"
)

// reconcileTick is the self-heal / heartbeat ticker cadence. Self-heal needs a
// sub-Grace cadence so a dangling follow resets on time even absent cluster
// events; the heartbeat keys off its own elapsed check.
const reconcileTick = 1 * time.Second

// Run launches the single reconcile goroutine: it ranges Cluster.Subscribe() and
// a 1 s ticker, and on each wake re-points the local subscriber/sink/clock at the
// current master, runs self-heal, tears down a session this node should no longer
// host, and (master, while playing) refreshes the playback heartbeat. Returns
// when ctx is cancelled or Close is called.
func (e *Engine) Run(ctx context.Context) {
	changes := e.p.Cluster.Subscribe()
	t := time.NewTicker(reconcileTick)
	defer t.Stop()

	e.reconcile() // initial pass

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.done:
			return
		case <-changes:
			e.reconcile()
		case <-t.C:
			e.reconcile()
		}
	}
}

// reconcile is one pass of the watch loop. Takes e.mu for the duration except
// across a session teardown halt (which is done lock-free, then re-locked).
func (e *Engine) reconcile() {
	now := e.now()
	snap := e.p.Cluster.Snapshot()
	mv := myGroup(snap, e.self)

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	if !mv.found {
		// Self not yet derived (transient): skip re-point/heal, no panic.
		e.mu.Unlock()
		return
	}

	// Log observed composition changes (the group I master + my player target).
	e.logCompositionLocked(mv)

	// Mid-session codec renegotiation (D33) for the group THIS node SOURCES: a live
	// session whose effective codec is no longer supported by all of MY group's
	// players downgrades to pcm in place. Run BEFORE driving the player so a gen bump
	// is picked up the same tick. A live session implies we source group(self).
	if e.sess != nil && !e.sess.paused.Load() {
		e.renegotiateLocked(snap, mv)
	}

	// Drive THIS node's PLAYER at its target group — group(self.Following) — which is
	// independent of what it sources (D49+ crosswise). Idle (Detach) when Following is
	// Zero / dead / unknown. When the target is self's own group, the live session
	// state is used (the snapshot's playback record lags).
	e.drivePlayerLocked(mv)

	// Heartbeat (D28): while sourcing, every Heartbeat interval.
	if e.sess != nil && !now.Before(e.lastBeat.Add(e.p.Heartbeat)) {
		e.p.Cluster.SetPlayback(e.sess.groupID, e.sess.playbackRecord(now, e.p.Source.Stats()))
		e.lastBeat = now
	}

	// 1 Hz playing stats: one INFO line per active side per second.
	e.logPlayingStatsLocked(mv, e.sess != nil, now)

	e.mu.Unlock()
}

// drivePlayerLocked points the local player at this node's TARGET group
// (group(self.Following)) — what its speakers play — independent of what it sources
// (D49+). Idle (Detach) when there is no live target. When the target is self's own
// group, it trusts the live session (the snapshot's playback record lags). Caller
// holds e.mu.
func (e *Engine) drivePlayerLocked(mv myView) {
	if !mv.hasTarget {
		// The PLAYER is idle (Following Zero / dead): the speakers play nothing. But
		// this node still MASTERS its own group (1:1) and must be able to SOURCE it,
		// which needs master-time — so keep the clock follower synced to its own clock
		// server over the NORMAL network (repointLocked → dialIP(self) → the node's
		// real advertised address, the same UDP clock probes any member uses; no
		// loopback shortcut) while leaving the sink disarmed (playing=false → Sync, no
		// Subscribe/arm). An idle player must not starve the master's own clock, else
		// Play's waitForClock times out → the misleading "not_master".
		e.repointLocked(e.self, e.curGen, mv.group.Settings.Transport, false)
		return
	}
	master := mv.target.Master
	transport := mv.target.Settings.Transport
	playing := mv.target.Playback.State == "playing"
	gen := e.curGen
	if master == e.self {
		// Playing my own group: trust my live session, not the lagging record.
		if e.sess != nil && !e.sess.paused.Load() {
			gen = e.sess.gen.Load()
			playing = true
		} else {
			playing = false
		}
	}
	e.repointLocked(master, gen, transport, playing)
}

// repointLocked arms the three local consumers (subscriber, sink, clock) for the
// given master + generation when they changed since last time (§3.2). The master
// dials itself via loopback like any member (§8.2). Caller holds e.mu; the calls
// are all non-blocking target updates.
func (e *Engine) repointLocked(master id.ID, gen uint32, transport string, playing bool) {
	if master.IsZero() {
		return
	}
	if e.haveCur && e.curMaster == master && e.curGen == gen && e.curPlaying == playing {
		return // unchanged: idempotent, no churn
	}
	// Generations are PER-MASTER counters: a new master's session may carry a
	// LOWER gen than our previous master's (or our own) last one. Never carry
	// the old floor across a master change — subscribe/arm at 0 and let the
	// first frame / RECONFIG establish the new master's gen (the client window
	// and the deliver-side sink re-arm both re-anchor upward).
	if e.haveCur && e.curMaster != master && master != e.self {
		gen = 0
	}

	ip := e.dialIP(master)
	if !ip.IsValid() {
		// No address yet (cold peer with no observations / no CIDRs). Try again
		// next reconcile; do not record the (master,gen) as armed.
		return
	}

	var srcPort, streamPort int
	for _, n := range e.snapshotNodes() {
		if n.ID == master {
			srcPort = n.SourcePort
			streamPort = n.StreamPort
			break
		}
	}
	if srcPort == 0 || streamPort == 0 {
		return
	}

	srcAddr := netip.AddrPortFrom(ip, uint16(srcPort))
	clkAddr := netip.AddrPortFrom(ip, uint16(streamPort))

	// Drive the local player (D61). The clock follower tracks the master ALWAYS
	// (cheap; keeps every member synced and ready) — Attach points it as part of
	// playing, Sync keeps it warm while idle. Stream subscription + sink arming are
	// session-gated. (Same SetMaster→Reset→Subscribe / SetMaster→Unsubscribe→Disarm
	// ordering as before the split.)
	if playing {
		e.player.Attach(playback.Attach{
			Source:    srcAddr,
			Clock:     clkAddr,
			Gen:       gen,
			Transport: stream.ParseTransport(transport),
		})
	} else {
		e.player.Sync(clkAddr, gen)
		if e.haveCur && e.curPlaying {
			// Session ended: leave the source politely (BYE) and disarm playout
			// cleanly (no starvation warnings).
			e.player.Detach()
		}
	}

	e.curMaster = master
	e.curGen = gen
	e.curPlaying = playing
	e.haveCur = true
}

// dialIP returns the best dial IP for master, substituting loopback when this
// node IS the master (DialCandidates may be empty on a loopback-only host).
func (e *Engine) dialIP(master id.ID) netip.Addr {
	if cands := e.p.Cluster.DialCandidates(master); len(cands) > 0 {
		return cands[0]
	}
	if master == e.self {
		return netip.AddrFrom4([4]byte{127, 0, 0, 1})
	}
	return netip.Addr{}
}

// snapshotNodes fetches the current node views (for port lookup). Small helper so
// repointLocked stays readable; called under e.mu but Snapshot has its own lock.
func (e *Engine) snapshotNodes() []nodeView {
	snap := e.p.Cluster.Snapshot()
	out := make([]nodeView, 0, len(snap.Nodes))
	for _, n := range snap.Nodes {
		out = append(out, nodeView{ID: n.ID, SourcePort: n.SourcePort, StreamPort: n.StreamPort})
	}
	return out
}

type nodeView struct {
	ID         id.ID
	SourcePort int
	StreamPort int
}
