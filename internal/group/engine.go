package group

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"ondaire/internal/clock"
	"ondaire/internal/contracts"
	"ondaire/internal/id"
)

// Typed errors surfaced to the API (I), which maps them to HTTP responses.
var (
	ErrNoOpus      = errors.New("group: opus codec not supported") // §8.3/D33
	ErrBadSettings = errors.New("group: invalid group settings")   // §9.1
	ErrNotSynced   = errors.New("group: self not derived yet, retry")
	ErrNotPlaying  = errors.New("group: nothing is playing") // D39 pause
	ErrNotPaused   = errors.New("group: not paused")         // D39 resume
	ErrNotSeekable = errors.New("group: source not seekable")
	ErrClosed      = errors.New("group: engine closed")
)

// Params bundles everything H needs, injected by main (K).
type Params struct {
	Cluster Cluster                // C: read + owner setters + DialCandidates
	Media   MediaFactory           // D: scheme-keyed media-source factory (§6.1)
	Opus    OpusFactory            // D: opus encoder factory (D33); may be nil (no opus)
	Source  SourceServer           // G internal/source: master-side fan-out
	Caps    contracts.Capabilities // this node's own caps (codec gating, §8.3)
	Log     *slog.Logger

	// PersistFollowing persists this node's follow target to node.json (D45),
	// called at EVERY site the engine writes cluster.SetFollowing (follow,
	// unfollow). nil-safe no-op (tests). K wires it to the config store's
	// SetFollowing.
	PersistFollowing func(id.ID)

	// Knobs (defaults applied in New when zero):
	LeadMs    int           // source release lead, default contracts.DefaultLeadMs (§8.2)
	Heartbeat time.Duration // playback position/SourceStats refresh, default 5 s (D28)

	// now is the time source (test seam). Defaults to time.Now.
	now func() time.Time

	// nowMaster is the master-clock anchor (test seam). The master OWNS the clock,
	// so master-time now == clock.MonoNow() exactly. Defaults to clock.MonoNow.
	nowMaster func() int64
}

// Engine is the group brain. One per node. One mutex guards all mutable fields.
type Engine struct {
	p         Params
	log       *slog.Logger
	self      id.ID
	now       func() time.Time
	nowMaster func() int64

	mu sync.Mutex

	// playback (master-only)
	sess *session // current session, nil = idle
	gen  uint32   // monotonic per-node session generation (§8.4); never reused

	lastBeat time.Time // last heartbeat SetPlayback (master, while playing)

	// observed composition (for logging member + play-target changes)
	prevTarget  id.ID // last play target (group(following) master); Zero = idle
	prevMembers map[id.ID]bool
	havePrev    bool

	// 1 Hz playing-stats throttle (one INFO line per second per side)
	lastStats time.Time

	closed bool
	done   chan struct{}
	wg     sync.WaitGroup
}

// New builds an Engine and applies knob defaults. Starts no goroutines.
func New(p Params) *Engine {
	if p.Log == nil {
		p.Log = slog.Default()
	}
	if p.LeadMs == 0 {
		p.LeadMs = contracts.DefaultLeadMs
	}
	if p.Heartbeat == 0 {
		p.Heartbeat = 5 * time.Second
	}
	if p.now == nil {
		p.now = time.Now
	}
	if p.nowMaster == nil {
		p.nowMaster = clock.MonoNow
	}
	self := p.Cluster.Self()
	return &Engine{
		p:         p,
		log:       p.Log.With("comp", "group"),
		self:      self,
		now:       p.now,
		nowMaster: p.nowMaster,
		done:      make(chan struct{}),
	}
}

// Close stops the reconcile goroutine, halts any running session (without
// rewriting the replicated doc, §3.4), and returns. Idempotent.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	close(e.done)
	e.stopLocked() // halts current session if any (releases+re-takes e.mu)
	e.mu.Unlock()

	e.wg.Wait()
	return nil
}

// setFollowing writes the replicated player-target (C) AND persists it to
// node.json (D45) so the node restores its target on restart. The two writes stay
// in lockstep at every follow/unfollow site. The persist hook is nil-safe.
func (e *Engine) setFollowing(target id.ID) {
	e.p.Cluster.SetFollowing(target)
	if e.p.PersistFollowing != nil {
		e.log.Debug("persisting following", "target", target.String())
		e.p.PersistFollowing(target)
	}
}

// Follow sets THIS node's player target (D49+): its speakers play group(target).
// target == self ⇒ play own group; any id allowed (no master-only rule).
func (e *Engine) Follow(target id.ID) error { return e.follow(target) }

// Unfollow sets THIS node's player idle (Following = Zero): it plays nothing.
func (e *Engine) Unfollow() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	e.setFollowing(id.Zero)
	e.log.Info("unfollowing (now solo)", "reason", "user")
	return nil
}

// Settings returns the effective group settings for this node's group.
func (e *Engine) Settings() contracts.GroupSettings {
	snap := e.p.Cluster.Snapshot()
	mv := myGroup(snap, e.self)
	if !mv.found {
		return defaultSettings()
	}
	return fillDefaults(mv.group.Settings)
}

// Group returns this node's current derived group view (for /api/status §9.1).
func (e *Engine) Group() contracts.GroupView {
	snap := e.p.Cluster.Snapshot()
	mv := myGroup(snap, e.self)
	return mv.group // zero value when not yet derived
}

// SourceStats returns the live source stats when this node runs a session, plus
// ok=false when idle (for /api/status §9.1/D19).
func (e *Engine) SourceStats() (contracts.SourceStats, bool) {
	e.mu.Lock()
	active := e.sess != nil
	e.mu.Unlock()
	if !active {
		return contracts.SourceStats{}, false
	}
	return e.p.Source.Stats(), true
}
