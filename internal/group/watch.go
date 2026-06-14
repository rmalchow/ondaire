package group

import (
	"context"
	"time"
)

// reconcileTick is the heartbeat / composition-logging ticker cadence. The
// heartbeat keys off its own elapsed check; the tick also drives mid-session
// codec renegotiation and the 1 Hz playing-stats line absent cluster events.
const reconcileTick = 1 * time.Second

// Run launches the single reconcile goroutine: it ranges Cluster.Subscribe() and
// a 1 s ticker, and on each wake logs composition changes, runs mid-session codec
// renegotiation, and (master, while playing) refreshes the playback heartbeat.
// Returns when ctx is cancelled or Close is called.
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

// reconcile is one pass of the watch loop, run on each cluster change and tick.
// The engine is a pure PRODUCER: it logs composition changes, runs mid-session
// codec renegotiation, refreshes the playback heartbeat while sourcing, and emits
// the 1 Hz playing-stats line. This node's own PLAYER is driven over the control
// plane by its Driver (ATTACH/DETACH), identically to a remote player — reconcile
// no longer points any subscriber/sink/clock. Takes e.mu for the duration.
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
		// Self not yet derived (transient): nothing to do, no panic.
		e.mu.Unlock()
		return
	}

	// Log observed composition changes (the group I master + my player target).
	e.logCompositionLocked(mv)

	// Mid-session codec renegotiation (D33) for the group THIS node SOURCES: a live
	// session whose effective codec is no longer supported by all of MY group's
	// players downgrades to pcm in place. A live session implies we source group(self).
	if e.sess != nil && !e.sess.paused.Load() {
		e.renegotiateLocked(snap, mv)
	}

	// Heartbeat (D28): while sourcing, every Heartbeat interval.
	if e.sess != nil && !now.Before(e.lastBeat.Add(e.p.Heartbeat)) {
		e.p.Cluster.SetPlayback(e.sess.groupID, e.sess.playbackRecord(now, e.p.Source.Stats()))
		e.lastBeat = now
	}

	// 1 Hz playing stats: one INFO line per active side per second.
	e.logPlayingStatsLocked(mv, e.sess != nil, now)

	e.mu.Unlock()
}
