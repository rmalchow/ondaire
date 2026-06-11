package sink

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"ensemble/internal/contracts"
)

// The resilient backend is the default output: a self-healing failover chain
// over every real output the host offers (the configured device first). It opens
// the first candidate that works, rotates to the next on failure, and — after a
// few full sweeps with nothing working — backs off and rests (discarding audio)
// before trying the whole chain again. A UI device override re-orders the chain
// (the override may itself fail, in which case the chain carries on past it); a
// test tone forces an immediate retry out of the rested state.
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

// candidate names one output to try in the failover chain.
type candidate struct {
	kind   string // "alsa" | "exec" — reported to the UI as the live backend kind
	arg    string // alsa device id ("default"/"hw:0,0"), or exec tool name
	label  string // human label for logs and the UI
	openFn func(*slog.Logger) (contracts.Backend, error)
}

func (c candidate) open(log *slog.Logger) (contracts.Backend, error) {
	return c.openFn(log)
}

// alsaCandidate / execCandidate construct the real openers (split out so tests can
// inject fakes via the candidate.openFn field directly).
func alsaCandidate(device string) candidate {
	return candidate{kind: "alsa", arg: device, label: "alsa(" + device + ")",
		openFn: func(log *slog.Logger) (contracts.Backend, error) { return newAlsaBackend(device, log) }}
}

func execCandidate(tool string) candidate {
	return candidate{kind: "exec", arg: tool, label: "exec(" + tool + ")",
		// No internal respawn: a death must surface so the chain rotates.
		openFn: func(log *slog.Logger) (contracts.Backend, error) { return newExecBackendTool(tool, false, log) }}
}

// resilientBackend implements contracts.Backend (+ Flusher / DelayReporter /
// LatencyReporter, forwarded to the live candidate). Single mutex; the blocking
// device write happens unlocked (the sink already serializes Write).
type resilientBackend struct {
	mu      sync.Mutex
	log     *slog.Logger
	cands   []candidate // failover chain, preferred first; never includes null
	idx     int         // index of the candidate `active` came from / next to try
	active  contracts.Backend
	since   time.Time // when `active` opened
	fails   int       // consecutive failures (open or write) since the last stable success
	backoff time.Duration
	restAt  time.Time // while now < restAt: discard (resting)
	closed  bool
	discard contracts.Backend // null backend, used while resting or when the chain is empty

	now           func() time.Time
	onActive      func(kind string) // optional: report the live backend kind (UI)
	pendingReport string            // kind to report after the lock is dropped ("" = nothing)
}

func newResilientBackend(cands []candidate, log *slog.Logger) *resilientBackend {
	return &resilientBackend{
		log:     log.With("backend", "auto"),
		cands:   cands,
		backoff: resilientBaseBackoff,
		discard: newNullBackend(),
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
		return fmt.Errorf("sink: backend closed")
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

	if err := ab.Write(frame); err == nil {
		r.mu.Lock()
		if r.active == ab && r.fails > 0 && r.now().Sub(r.since) >= resilientStableAfter {
			r.log.Info("output stable", "candidate", r.cands[r.idx].label)
			r.fails, r.backoff = 0, resilientBaseBackoff
		}
		r.mu.Unlock()
		return nil
	}
	// Failure: drop this candidate and rotate. Swallow the error — the chain
	// self-heals, and surfacing it would re-flood the sink's "backend write failed".
	r.mu.Lock()
	if r.active == ab {
		_ = ab.Close()
		r.active = nil
		r.log.Warn("output candidate failed; rotating", "candidate", r.cands[r.idx].label)
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
		b, err := c.open(r.log)
		if err == nil {
			r.active = b
			r.since = r.now()
			r.pendingReport = c.kind
			r.log.Info("output opened", "candidate", c.label)
			return
		}
		r.log.Debug("output candidate failed to open", "candidate", c.label, "err", err)
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
	r.backoff = resilientBaseBackoff
	r.fails, r.idx = 0, 0
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	r.log.Info("output revive requested")
}

// SetPreferred re-orders the chain so the ALSA candidate for `device` is tried
// first (the UI device override, D37). The override may itself fail, in which case
// the chain carries on to the others. An empty device prefers the system default.
func (r *resilientBackend) SetPreferred(device string) {
	if device == "" {
		device = "default"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Pull an existing alsa candidate for this device to the front, or synthesize one.
	pref := alsaCandidate(device)
	rest := make([]candidate, 0, len(r.cands))
	for _, c := range r.cands {
		if c.kind == "alsa" && c.arg == device {
			pref = c
			continue
		}
		rest = append(rest, c)
	}
	r.cands = append([]candidate{pref}, rest...)
	r.idx, r.fails = 0, 0
	r.restAt = time.Time{}
	r.backoff = resilientBaseBackoff
	if r.active != nil {
		_ = r.active.Close()
		r.active = nil
	}
	r.log.Info("output device override; preferring", "device", device)
}

// Flush forwards to the live candidate when it is a Flusher.
func (r *resilientBackend) Flush() {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if fl, ok := ab.(contracts.Flusher); ok {
		fl.Flush()
	}
}

// DeviceDelay forwards to the live candidate (ALSA reports it; exec/null do not).
func (r *resilientBackend) DeviceDelay() (int64, bool) {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if dr, ok := ab.(contracts.DelayReporter); ok {
		return dr.DeviceDelay()
	}
	return 0, false
}

// ConfiguredLatencyNs forwards to the live candidate, else 0.
func (r *resilientBackend) ConfiguredLatencyNs() int64 {
	r.mu.Lock()
	ab := r.active
	r.mu.Unlock()
	if lr, ok := ab.(contracts.LatencyReporter); ok {
		return lr.ConfiguredLatencyNs()
	}
	return 0
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

// buildCandidates assembles the failover chain from the requested spec/device and
// the host's enumerated outputs. Order: the explicit/override device first, then
// every ALSA device (system default, then each hw:C,D), then every player tool on
// $PATH. Dedup-stable. null/file specs are handled by the caller (no chain).
func buildCandidates(spec, device string) []candidate {
	var cands []candidate
	seen := map[string]bool{}
	add := func(c candidate) {
		key := c.kind + "|" + c.arg
		if seen[key] {
			return
		}
		seen[key] = true
		cands = append(cands, c)
	}

	name, arg := splitSpec(spec)
	switch name {
	case "alsa":
		dev := arg
		if dev == "" {
			dev = device
		}
		if dev == "" {
			dev = "default"
		}
		add(alsaCandidate(dev))
	case "exec":
		if arg != "" {
			add(execCandidate(arg))
		}
	}
	// The configured device (D37) takes priority over auto-enumeration.
	if device != "" {
		add(alsaCandidate(device))
	}
	// Every ALSA playback device (default + hw:C,D). Empty when libasound absent.
	for _, d := range ListOutputDevices() {
		add(alsaCandidate(d.ID))
	}
	// Every player tool actually present, in preference order.
	for _, t := range execTools {
		if _, _, ok := lookExecToolNamed(t.name); ok {
			add(execCandidate(t.name))
		}
	}
	return cands
}

// candidateLabels renders the chain for a startup log line.
func candidateLabels(cands []candidate) string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.label
	}
	return strings.Join(out, " → ")
}
