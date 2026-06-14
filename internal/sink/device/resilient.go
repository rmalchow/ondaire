package device

import (
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"
)

// The resilient backend is the default output: a self-healing failover chain over
// every real output the host offers (the configured device first). It opens the
// first candidate that works, rotates to the next on failure, and — after a few
// full sweeps with nothing working — backs off and rests (discarding audio) before
// trying the whole chain again. A UI device override re-orders the chain (the
// override may itself fail, in which case the chain carries on past it); a test
// tone forces an immediate retry out of the rested state.
const (
	// resilientStableAfter is how long a candidate must keep accepting writes
	// before it counts as "working" and the failure/backoff counters reset. A
	// flapping player (pw-play with no session accepts one frame, then dies) never
	// reaches it, so the chain correctly progresses to backoff.
	resilientStableAfter = 2 * time.Second
	resilientBaseBackoff = 2 * time.Second
	resilientMaxBackoff  = 60 * time.Second
	// resilientMaxSweeps is how many full passes over the chain may fail before we
	// rest ("back off if things go wrong more than 3 times").
	resilientMaxSweeps = 3
)

// resilientBackend implements device.Sink and forwards/aggregates the optional
// capabilities (Flusher / DelayReporter / LatencyReporter / Interrupter /
// StatsReporter / DeviceSelector / Reviver / ActiveReporter) to its LIVE
// candidate. Single mutex; the blocking device write happens UNLOCKED (the engine
// already serializes Write) so Interrupt can reach the candidate mid-write.
type resilientBackend struct {
	mu      sync.Mutex
	log     *slog.Logger
	cands   []Candidate // failover chain, preferred first; never includes null
	idx     int         // index of the candidate `active` came from / next to try
	active  Sink
	since   time.Time // when `active` opened
	fails   int       // consecutive failures (open or write) since the last stable success
	backoff time.Duration
	restAt  time.Time // while now < restAt: discard (resting)
	closed  bool
	discard Sink // null sink, used while resting or when the chain is empty

	rotations uint64 // total candidate switches, for DeviceStats (wrapper-only)
	resting   bool   // currently in backoff, for DeviceStats (wrapper-only)

	now           func() time.Time
	onActive      func(kind string) // optional: report the live backend kind (UI)
	pendingReport string            // kind to report after the lock is dropped ("" = nothing)
}

// newResilientBackend builds the failover wrapper over a non-empty candidate chain.
// The null discard sink is opened via the registry so this file stays backend-agnostic.
func newResilientBackend(cands []Candidate, log *slog.Logger) *resilientBackend {
	discard, _ := openFactory("null", "", log)
	return &resilientBackend{
		log:     log.With("backend", "auto"),
		cands:   cands,
		backoff: resilientBaseBackoff,
		discard: discard,
		now:     time.Now,
	}
}

// OnActive registers a callback fired with the live backend kind ("alsa"|"exec")
// whenever the active candidate changes, so the cluster record / UI can show what
// is actually playing. Set once at wiring time.
func (r *resilientBackend) OnActive(fn func(kind string)) {
	r.mu.Lock()
	r.onActive = fn
	r.mu.Unlock()
}

func (r *resilientBackend) Write(frame []byte) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("device: backend closed")
	}
	if len(r.cands) == 0 {
		// No real output on this host: behave like null (permanent discard).
		r.mu.Unlock()
		return r.discard.Write(frame)
	}
	if r.now().Before(r.restAt) {
		r.mu.Unlock()
		return r.discard.Write(frame) // resting after repeated failure
	}
	if !r.restAt.IsZero() {
		r.restAt = time.Time{} // backoff elapsed: wake and retry the chain from the top
		r.resting = false
		r.idx, r.fails = 0, 0
		r.log.Info("output backoff elapsed; retrying outputs")
	}
	if r.active == nil {
		r.openFromLocked()
	}
	ab := r.active
	report := r.takeReportLocked()
	r.mu.Unlock()

	if report != "" && r.onActive != nil {
		r.onActive(report)
	}
	if ab == nil {
		return r.discard.Write(frame) // nothing opened (now resting) — discard this frame
	}

	// UNLOCKED blocking write: this IS the rate pacer, and keeping the lock off the
	// hot path lets a concurrent Interrupt reach the candidate (see As / Interrupt).
	if err := ab.Write(frame); err == nil {
		r.mu.Lock()
		if r.active == ab && r.fails > 0 && r.now().Sub(r.since) >= resilientStableAfter {
			r.log.Info("output stable", "candidate", r.cands[r.idx].Label)
			r.fails, r.backoff = 0, resilientBaseBackoff
		}
		r.mu.Unlock()
		return nil
	}
	// Failure: drop this candidate and rotate. Swallow the error — the chain
	// self-heals, and surfacing it would re-flood the engine's "write failed".
	r.mu.Lock()
	if r.active == ab {
		_ = ab.Close()
		r.active = nil
		r.log.Warn("output candidate failed; rotating", "candidate", r.cands[r.idx].Label)
		r.advanceLocked()
	}
	r.mu.Unlock()
	return nil
}

// openFromLocked opens candidates starting at r.idx until one opens; a full sweep
// with nothing opening enters the rested state. Caller holds r.mu.
func (r *resilientBackend) openFromLocked() {
	for i := 0; i < len(r.cands); i++ {
		c := r.cands[r.idx]
		b, err := c.Open(r.log)
		if err == nil {
			r.active = b
			r.since = r.now()
			r.pendingReport = c.Kind
			r.rotations++
			r.log.Info("output opened", "candidate", c.Label)
			return
		}
		r.log.Debug("output candidate failed to open", "candidate", c.Label, "err", err)
		r.advanceLocked()
		if !r.restAt.IsZero() {
			return // entered rest mid-sweep
		}
	}
}

// advanceLocked records a failure and steps to the next candidate, entering the
// rested state once enough full sweeps have failed. Caller holds r.mu.
func (r *resilientBackend) advanceLocked() {
	r.fails++
	r.idx = (r.idx + 1) % len(r.cands)
	if r.fails >= len(r.cands)*resilientMaxSweeps {
		r.enterRestLocked()
	}
}

func (r *resilientBackend) enterRestLocked() {
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	r.restAt = r.now().Add(r.backoff)
	r.resting = true
	r.log.Warn("all outputs failing; backing off", "restMs", r.backoff.Milliseconds())
	r.backoff *= 2
	if r.backoff > resilientMaxBackoff {
		r.backoff = resilientMaxBackoff
	}
	r.fails, r.idx = 0, 0
}

func (r *resilientBackend) takeReportLocked() string {
	s := r.pendingReport
	r.pendingReport = ""
	return s
}

// Revive clears the rested/backoff state and retries the chain from the top
// immediately — driven by the UI test tone, so an operator can poke a node that
// has given up. No-op when closed.
func (r *resilientBackend) Revive() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.restAt = time.Time{}
	r.resting = false
	r.backoff = resilientBaseBackoff
	r.fails, r.idx = 0, 0
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	r.log.Info("output revive requested")
}

// SetPreferred rebuilds the chain so the candidate for `device` is tried first
// (the UI device override, D37). It re-runs Candidates(device) so the chosen
// device leads; the override may itself fail, in which case the chain carries on to
// the others. Returns true: the resilient wrapper always honours a selection.
func (r *resilientBackend) SetPreferred(device string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cands = Candidates(device)
	r.idx, r.fails = 0, 0
	r.restAt = time.Time{}
	r.resting = false
	r.backoff = resilientBaseBackoff
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	r.log.Info("output device override; preferring", "device", deviceLabel(device))
	return true
}

// Flush forwards to the live candidate when it is a Flusher.
func (r *resilientBackend) Flush() {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if fl, ok := Query[Flusher](ab); ok {
		fl.Flush()
	}
}

// Interrupt forwards to the live candidate when it is an Interrupter, aborting its
// in-flight blocking Write so Close/Reset stay snappy.
func (r *resilientBackend) Interrupt() {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if in, ok := Query[Interrupter](ab); ok {
		in.Interrupt()
	}
}

// Delay forwards to the live candidate (ALSA reports it; exec/null do not). ok=false
// when nothing is live or the candidate is opaque.
func (r *resilientBackend) Delay() (int64, bool) {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if dr, ok := Query[DelayReporter](ab); ok {
		return dr.Delay()
	}
	return 0, false
}

// ConfiguredLatencyNs forwards to the live candidate, else 0.
func (r *resilientBackend) ConfiguredLatencyNs() int64 {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if lr, ok := Query[LatencyReporter](ab); ok {
		return lr.ConfiguredLatencyNs()
	}
	return 0
}

// DeviceStats returns the LIVE candidate's stats with the wrapper-only fields
// overlaid: the live kind, the total candidate switches (Rotations), whether the
// chain is currently resting (Resting) and the current backoff window (BackoffMs).
// When nothing is live (resting / empty chain) the leaf stats are zero and only the
// wrapper fields carry information.
func (r *resilientBackend) DeviceStats() DeviceStats {
	r.mu.Lock()
	ab := r.active
	rotations := r.rotations
	resting := r.resting
	backoffMs := r.backoff.Milliseconds()
	r.mu.Unlock()

	var st DeviceStats
	if sr, ok := Query[StatsReporter](ab); ok {
		st = sr.DeviceStats()
	}
	if st.Kind == "" {
		st.Kind = "null"
	}
	st.Rotations = rotations
	st.Resting = resting
	st.BackoffMs = backoffMs
	return st
}

func (r *resilientBackend) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	return r.discard.Close()
}

// As is the capability escape hatch read by device.Query: it routes a Query[T] on
// the wrapper to the LIVE candidate's T. Modeled on errors.As — `target` is a
// non-nil pointer to an interface type; if the live candidate implements that
// interface, As stores it and returns true.
//
// This is load-bearing for the servo: device.Query[device.DelayReporter](sink) on
// the wrapper reaches the live ALSA candidate's Delay() THROUGH As, so the phase
// probe sees real hardware latency. It also future-proofs the wrapper — a NEW
// capability flows through with no edit here. (The typed forwards above remain so
// the wrapper additionally aggregates wrapper-only state, e.g. DeviceStats.)
func (r *resilientBackend) As(target any) bool {
	if target == nil {
		return false
	}
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return false
	}
	typ := val.Elem().Type()
	if typ.Kind() != reflect.Interface {
		return false
	}

	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if ab == nil {
		return false
	}
	abVal := reflect.ValueOf(ab)
	if abVal.Type().Implements(typ) {
		val.Elem().Set(abVal)
		return true
	}
	return false
}
