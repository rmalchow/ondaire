package group

import (
	"time"

	"ensemble/internal/id"
)

// logCompositionLocked diffs this node's freshly-derived view against the previous
// one and logs player join/leave (of the group it MASTERS) and changes to its own
// PLAYER target (the group its speakers play). Caller holds e.mu.
func (e *Engine) logCompositionLocked(mv myView) {
	members := make(map[id.ID]bool, len(mv.group.Members))
	for _, m := range mv.group.Members {
		members[m] = true
	}
	target := id.Zero
	if mv.hasTarget {
		target = mv.target.Master
	}

	if !e.havePrev {
		e.havePrev = true
		e.prevTarget = target
		e.prevMembers = members
		e.log.Info("group composition",
			"group", mv.group.ID.String(), "players", len(members), "playTarget", targetLabel(target))
		return
	}

	for m := range members {
		if !e.prevMembers[m] {
			e.log.Info("group player joined", "group", mv.group.ID.String(), "player", m.String())
		}
	}
	for m := range e.prevMembers {
		if !members[m] {
			e.log.Info("group player left", "group", mv.group.ID.String(), "player", m.String())
		}
	}
	if target != e.prevTarget {
		e.log.Info("play target changed", "from", targetLabel(e.prevTarget), "to", targetLabel(target))
	}

	e.prevTarget = target
	e.prevMembers = members
}

// targetLabel renders a play target for logs: "idle" for the zero target.
func targetLabel(t id.ID) string {
	if t.IsZero() {
		return "idle"
	}
	return t.String()
}

// logPlayingStatsLocked emits the master-side 1 Hz playing-stats line while this
// node SOURCES a session. One INFO line per second; silent when idle. Caller holds
// e.mu. (The member side is logged from K's wiring.)
func (e *Engine) logPlayingStatsLocked(mv myView, sourcing bool, now time.Time) {
	if e.sess == nil || !sourcing {
		return
	}
	if now.Before(e.lastStats.Add(time.Second)) {
		return
	}
	e.lastStats = now

	st := e.p.Source.Stats()
	e.log.Info("playing",
		"side", "master",
		"gen", e.sess.gen.Load(),
		"released", st.Released,
		"clients", st.Clients,
		"parity", st.Parity,
		"restarts", st.Restarts,
		"primes", st.Primes,
	)
}
