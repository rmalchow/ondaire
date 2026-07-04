// Package sink is the playout pipeline: jitter buffer, pts-driven scheduler,
// continuous rate servo + 4-tap resampler, live volume gain, and the output
// device port (internal/sink/device). Piece E.
package sink

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"ondaire/internal/clock"
	"ondaire/internal/contracts"
	"ondaire/internal/sink/device"
	"ondaire/internal/stream"
)

// monotoNow returns the local monotonic clock in nanoseconds — clock.MonoNow,
// the SAME clock the follower measures offsets against. MasterToLocal output
// is only comparable to this clock; wall time (UnixNano) is off by the
// inter-process start-delta and is additionally non-monotonic under NTP steps.
func monotoNow() int64 { return clock.MonoNow() }

// Playout is the per-node sink: jitter buffer → resampler → gain → device, with
// the continuous phase-lock servo and the starvation watchdog. Implements
// contracts.Sink. One scheduler goroutine, one write-stall watcher, one mutex.
//
// The device's blocking Write is the rate pacer (PLAN-dac-pull-phase-lock): there
// is no deadline sleep in steady state. When the device exposes a phase probe
// (device.DelayReporter, i.e. ALSA's snd_pcm_delay), the servo locks the play head
// to the master clock by trimming the resample ratio; without one (exec/file/null),
// the ratio stays 1 and phase is set once at prime, the device's own clock pacing.
type Playout struct {
	mu              sync.Mutex
	jb              *jitterBuffer
	servo           *rateServo
	rs              *resampler
	gen             uint32
	armed           bool
	closed          bool
	toneBusy        bool // a TestTone writer goroutine is active
	stats           contracts.SinkStats
	lastPkt         int64 // local-ns of most recent accepted Push (watchdog)
	lastServoLog    int64 // local ns of the last 1Hz servo debug line
	lastUnderrunLog int64 // local ns of the last underrun (input-starved) warning

	// session state
	originSeq    uint64
	originPTS    int64
	originSet    bool
	dacPrimed    bool    // Phase-B prime done this session (phase aligned)
	fedAnchorPTS int64   // master PTS of the read cursor captured at the last (re)prime
	fedAnchorCon float64 // resampler consumedSamples() at that prime — fedPTS is measured RELATIVE to this
	//                      pair, so a re-anchor (equalize/delay change, re-arm) can't inject a phantom
	//                      phase offset from samples consumed before it (the v0.16 hardware bug).
	restartHit bool // RESTART already fired this starvation episode

	clock           contracts.Clock
	out             device.Sink
	delay           device.DelayReporter   // out's live queue (telemetry only), or nil
	latency         device.LatencyReporter // out's CONSTANT configured latency (the phase offset), or nil
	flush           device.Flusher         // out's flush, or nil
	interrupt       device.Interrupter     // out's blocking-write abort, or nil
	gain            *gainStage
	channel         *channelStage // dual-mono collapse (stereo|L|R); lock-free, applied after gain
	bufferNs        int64
	delayOffsetNs   int64 // output-delay calibration (D36, acoustic/room)
	equalizeDelayNs int64 // master-driven cross-room device-buffer equalization (D65)
	deviceLatencyNs int64 // backend's configured output latency (D63); stall-guard horizon
	watchdog        time.Duration
	restart         RestartFunc
	now             func() int64
	log             *slog.Logger

	silence      []byte
	writeStartNs atomic.Int64 // local ns a blocking Write started (0 = not writing); stall guard
	wake         chan struct{}
	done         chan struct{}
	wg           sync.WaitGroup
}

// New builds a Playout and starts its goroutines (idle until the first Reset).
// cfg.Backend and cfg.Clock must be non-nil.
func New(cfg Config) *Playout {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	log = log.With("comp", "sink")

	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	watchdog := cfg.Watchdog
	if watchdog <= 0 {
		watchdog = defaultWatchdog
	}
	bufferMs := cfg.BufferMs
	if bufferMs <= 0 {
		bufferMs = contracts.DefaultBufferMs
	}
	nowFn := cfg.now
	if nowFn == nil {
		nowFn = monotoNow
	}
	sc := cfg.servoCfg
	if sc.Kp <= 0 {
		sc = defaultServoConfig()
	}

	p := &Playout{
		jb:            newJitterBuffer(capacity),
		servo:         newRateServo(sc),
		rs:            newResampler(),
		gain:          newGainStage(cfg.Volume),
		channel:       newChannelStage(cfg.Channel),
		clock:         cfg.Clock,
		out:           cfg.Backend,
		bufferNs:      int64(bufferMs) * 1_000_000,
		delayOffsetNs: clampDelayNs(int64(cfg.OutputDelayMs) * 1_000_000),
		watchdog:      watchdog,
		restart:       cfg.Restart,
		now:           nowFn,
		log:           log,
		silence:       make([]byte, stream.FrameBytes),
		wake:          make(chan struct{}, 1),
		done:          make(chan struct{}),
	}
	// Optional device capabilities (the resilient wrapper implements each directly,
	// forwarding to whichever candidate is live; a leaf adapter implements only what
	// its hardware can do). The phase probe is the servo's only drift sensor.
	p.flush, _ = device.Query[device.Flusher](cfg.Backend)
	p.delay, _ = device.Query[device.DelayReporter](cfg.Backend)
	p.interrupt, _ = device.Query[device.Interrupter](cfg.Backend)
	if lr, ok := device.Query[device.LatencyReporter](cfg.Backend); ok {
		p.latency = lr
		p.deviceLatencyNs = lr.ConfiguredLatencyNs() // refreshed live in observeLocked (resilient opens lazily)
	}
	p.wg.Add(2)
	go p.loop()
	go p.watchWrites()
	return p
}

// Push enqueues a frame for playout (contracts.Sink). Non-blocking; drops+counts
// stale-gen / late frames; copies payload; signals the scheduler.
func (p *Playout) Push(gen uint32, seq uint64, pts int64, payload []byte) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if gen != p.gen {
		p.stats.StaleGen++
		p.mu.Unlock()
		return
	}
	if !p.armed {
		p.stats.StaleGen++
		p.mu.Unlock()
		return
	}
	if !p.originSet {
		p.jb.setOrigin(seq)
		p.originSeq = seq
		p.originPTS = pts
		p.originSet = true
	}
	if !p.jb.insert(seq, pts, payload) {
		p.stats.LateDrop++
	} else {
		p.lastPkt = p.now()
	}
	p.stats.Buffered = p.jb.len()
	p.mu.Unlock()
	p.signal()
}

// Reset arms the sink for a new generation: discards queued frames, resets servo
// + resampler, sets gen, clears per-session counters, re-establishes the seq/pts
// origin on the next Push.
func (p *Playout) Reset(gen uint32) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.jb.reset()
	p.servo.reset()
	p.rs.reset()
	p.gen = gen
	p.stats.Played = 0
	p.stats.Silence = 0
	p.stats.LateDrop = 0
	p.stats.StaleGen = 0
	p.stats.RatePPM = 0
	p.stats.PhaseErrNs = 0
	p.stats.Calibrated = false
	p.stats.Buffered = 0
	p.dacPrimed = false
	p.fedAnchorPTS = 0
	p.fedAnchorCon = 0
	p.restartHit = false
	p.originSet = false
	p.armed = true
	p.lastPkt = p.now()
	bufMs := p.bufferNs / 1_000_000
	delayMs := p.delayOffsetNs / 1_000_000
	p.mu.Unlock()
	p.log.Info("session armed", "gen", gen, "bufferMs", bufMs, "delayOffsetMs", delayMs)
	p.signal()
}

// SetBufferMs updates the playout lead live (D21/D23); takes effect on the next
// scheduled slot.
func (p *Playout) SetBufferMs(ms int) {
	if ms <= 0 {
		ms = contracts.DefaultBufferMs
	}
	p.mu.Lock()
	p.bufferNs = int64(ms) * 1_000_000
	p.mu.Unlock()
}

// SetGain sets the live software volume (0.0–1.0, D35; contracts.Sink).
// Lock-free: stores the gain stage's atomic target; the scheduler ramps over the
// next frame. Safe from any goroutine, no restart, applies on every backend.
func (p *Playout) SetGain(g float64) {
	if p.gain.currentTarget() == g {
		return // unchanged: silence the per-PATCH flood from a dragging slider
	}
	p.gain.setTarget(g)
	p.log.Info("gain changed", "volume", g)
}

// SetChannel sets the live playout channel mode ("stereo"|"L"|"R"). Lock-free:
// stores the channel stage's atomic mode; the next frame collapses to dual-mono
// (or passes stereo through). No re-anchor — it's a pure per-frame output
// transform, so sync/phase are unaffected. Safe from any goroutine.
func (p *Playout) SetChannel(ch string) {
	mode := parseChannel(ch)
	if p.channel.current() == mode {
		return
	}
	p.channel.set(ch)
	p.log.Info("channel changed", "channel", ch)
}

// SetDelayOffset sets the node's output-delay calibration in nanoseconds (D36;
// contracts.Sink). Under the mutex it stores the clamped offset, discards
// buffered frames (re-armed on the next Push), and fires the RestartFunc the
// starvation watchdog uses so G's subscriber issues a wire RESTART and the
// source burst re-primes under the new anchor.
func (p *Playout) SetDelayOffset(nanos int64) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.delayOffsetNs = clampDelayNs(nanos)
	p.jb.reset()
	p.originSet = false
	p.dacPrimed = false
	p.stats.Buffered = 0
	restart := p.restart
	offMs := p.delayOffsetNs / 1_000_000
	p.mu.Unlock()
	p.log.Info("delay offset changed; re-anchoring", "delayOffsetMs", offMs, "willRestart", restart != nil)
	if restart != nil {
		restart()
	}
}

// SetEqualizeDelay sets the master-driven cross-room equalization delay in
// nanoseconds (D65). This is a SEPARATE component from the D36 acoustic offset
// (SetDelayOffset): the deadline adds it, so the master can delay a faster room to
// match the slowest WITHOUT clobbering the node's own acoustic calibration. Clamped
// to ≥0 (it only ever delays). Re-anchors exactly like SetDelayOffset; a no-op when
// the value is unchanged so the master's 1 Hz re-assert never re-anchors (an audible
// glitch) — only a real change does.
func (p *Playout) SetEqualizeDelay(nanos int64) {
	if nanos < 0 {
		nanos = 0
	}
	nanos = clampDelayNs(nanos)
	p.mu.Lock()
	if p.closed || p.equalizeDelayNs == nanos {
		p.mu.Unlock()
		return
	}
	p.equalizeDelayNs = nanos
	p.jb.reset()
	p.originSet = false
	p.dacPrimed = false
	p.stats.Buffered = 0
	restart := p.restart
	eqMs := p.equalizeDelayNs / 1_000_000
	p.mu.Unlock()
	p.log.Info("equalize delay changed; re-anchoring", "equalizeMs", eqMs, "willRestart", restart != nil)
	if restart != nil {
		restart()
	}
}

// SwapBackend replaces the live output device (D37, §8.5): used when the node's
// selected output device changes. Under the mutex it closes the old device, sets
// the new one (re-resolving its capabilities), and logs. A brief audio blip is
// acceptable; the session/scheduler is NOT restarted. No-op (closing nb) if the
// sink is already closed.
func (p *Playout) SwapBackend(nb device.Sink) {
	if nb == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = nb.Close()
		return
	}
	old := p.out
	p.out = nb
	p.flush, _ = device.Query[device.Flusher](nb)
	p.delay, _ = device.Query[device.DelayReporter](nb)
	p.interrupt, _ = device.Query[device.Interrupter](nb)
	if lr, ok := device.Query[device.LatencyReporter](nb); ok {
		p.latency = lr
		p.deviceLatencyNs = lr.ConfiguredLatencyNs()
	} else {
		p.latency = nil
		p.deviceLatencyNs = 0
	}
	p.dacPrimed = false // re-prime phase against the new device
	p.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	p.log.Info("output backend swapped")
}

// PreferOutputDevice forwards a device override to the LIVE output backend when it
// supports re-ordering its failover chain (the resilient backend) — the UI device
// selection (D37). Always targets the current backend, so it is correct across a
// SwapBackend. Returns false when the live backend ignores devices.
func (p *Playout) PreferOutputDevice(dev string) bool {
	p.mu.Lock()
	out := p.out
	p.mu.Unlock()
	if sel, ok := device.Query[device.DeviceSelector](out); ok {
		return sel.SetPreferred(dev)
	}
	return false
}

// Disarm cleanly ends the local session (contracts.Sink): group went idle or the
// session stopped. Discards buffered frames and stops the scheduler with no
// starvation warnings. Idempotent; Reset re-arms.
func (p *Playout) Disarm() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !p.armed {
		return
	}
	p.armed = false
	p.dacPrimed = false
	p.jb.reset()
	p.servo.reset()
	p.rs.reset()
	p.stats.Buffered = 0
	if p.flush != nil {
		// Session over: drop whatever the device/player retains, or it audibly
		// replays at the next session's start (user-reported on leave+rejoin). The
		// flush (snd_pcm_drop / player respawn) also unblocks a parked Write.
		p.flush.Flush()
	}
	p.log.Info("playout disarmed (session ended)", "gen", p.gen)
	p.signal()
}

// ArmedGen reports the generation the sink is currently armed for and whether it is
// armed at all. The member-side deliver path uses it to decide when to re-arm.
func (p *Playout) ArmedGen() (gen uint32, armed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gen, p.armed
}

// Stats snapshots playout counters (contracts.Sink). Synced is read live from the
// clock; the device's phase probe / queue depth is merged from its DeviceStats.
func (p *Playout) Stats() contracts.SinkStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.stats
	s.Buffered = p.jb.len()
	s.RatePPM = p.servo.ratePPM()
	s.SamplesInjected, s.SamplesDropped = p.rs.sampleStats()
	if sr, ok := device.Query[device.StatsReporter](p.out); ok {
		if ds := sr.DeviceStats(); ds.QueueValid {
			s.DeviceDelayNs = ds.QueueNs
		}
	}
	_, ok := p.clock.MasterNow()
	s.Synced = ok
	return s
}

// DeviceStats returns the live device's rich telemetry (xruns, write errors,
// failover health, …) for the local /api/status debug view — a superset of what
// the frozen contracts.SinkStats carries across the wire. ok=false if the device
// reports none.
func (p *Playout) DeviceStats() (device.DeviceStats, bool) {
	p.mu.Lock()
	out := p.out
	p.mu.Unlock()
	if sr, ok := device.Query[device.StatsReporter](out); ok {
		return sr.DeviceStats(), true
	}
	return device.DeviceStats{}, false
}

// Close stops the goroutines and closes the device (contracts.Sink). Idempotent.
// It interrupts a parked blocking Write first so wg.Wait can't hang on a device
// that is slow (or wedged) inside Write.
func (p *Playout) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.done)
	intr := p.interrupt
	p.mu.Unlock()
	if intr != nil {
		intr.Interrupt()
	}
	p.wg.Wait()
	return p.out.Close()
}

// signal pokes the scheduler (non-blocking).
func (p *Playout) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// clampDelayNs clamps an output-delay offset to ±maxDelayMs.
func clampDelayNs(nanos int64) int64 {
	max := int64(maxDelayMs) * 1_000_000
	if nanos > max {
		return max
	}
	if nanos < -max {
		return -max
	}
	return nanos
}

// slotPTS returns the master-clock PTS for a seq, based on the session origin.
func (p *Playout) slotPTS(seq uint64) int64 {
	return p.originPTS + int64(seq-p.originSeq)*stream.FrameNanos
}

// deadlineLocal returns the local time at which a seq's content should reach the
// speaker: MasterToLocal(pts + buffer − delayOffset + equalize). Caller holds mu.
func (p *Playout) deadlineLocal(seq uint64) (int64, bool) {
	target := p.slotPTS(seq) + p.bufferNs - p.delayOffsetNs + p.equalizeDelayNs
	return p.clock.MasterToLocal(target)
}

// loop is the single scheduler goroutine. Once synced and primed it pulls one
// device-paced frame per iteration: feed the resampler from the jitter buffer,
// render at the servo's ratio, and let the blocking Write pace the loop.
func (p *Playout) loop() {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		fireRestart := p.checkStarvationLocked()
		if !p.armed || !p.jb.hasPending() {
			p.mu.Unlock()
			if fireRestart != nil {
				fireRestart()
			}
			if p.waitIdle() {
				return
			}
			continue
		}
		if fireRestart != nil {
			p.mu.Unlock()
			fireRestart()
			continue
		}

		// §7 unsynced gate: phase placement needs the master clock.
		if _, ok := p.clock.MasterNow(); !ok {
			p.mu.Unlock()
			if p.sleepInterruptible(5 * time.Millisecond) {
				return
			}
			continue
		}

		// Phase B: align the first frame's phase once per session.
		if !p.dacPrimed {
			p.mu.Unlock()
			if p.prime() {
				return
			}
			continue
		}

		// Phase C: feed + render under the lock (pure CPU; never races rs.reset),
		// then write unlocked (the blocking Write is the pacer), then observe.
		curGen := p.gen
		p.feedLocked()
		out := p.rs.process(p.servo.currentRatio())
		p.gain.apply(out)
		p.channel.apply(out) // dual-mono collapse (no-op for stereo); after gain, before device
		p.stats.Buffered = p.jb.len()
		p.mu.Unlock()

		p.writeStartNs.Store(p.now())
		err := p.out.Write(out)
		p.writeStartNs.Store(0)
		if err != nil {
			p.log.Warn("backend write failed", "err", err)
		}

		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if !p.armed || p.gen != curGen {
			p.mu.Unlock()
			continue // session changed under us; the next iteration re-primes
		}
		p.observeLocked()
		p.mu.Unlock()
	}
}

// prime performs Phase B once per session: skip overdue frames, then align so the
// first real frame reaches the speaker at its master-clock deadline. With a phase
// probe (ALSA) it pre-fills the device with silence (paced by the blocking write)
// until the next write would land in phase; without one it simply sleeps to the
// deadline (the device's own clock then paces Phase C). Caller must NOT hold the
// lock. Returns true if Close fired.
func (p *Playout) prime() (closed bool) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return true
	}
	if !p.armed || !p.jb.hasPending() {
		p.mu.Unlock()
		return false // re-evaluate at the loop top
	}
	gen := p.gen
	// Drop frames whose deadline already passed — they are unplayable; starting on
	// one would force the servo to chase a huge negative phase error.
	for p.jb.hasPending() {
		s0, ok := p.deadlineLocal(p.jb.nextSeq)
		if !ok {
			p.mu.Unlock()
			if p.sleepInterruptible(5 * time.Millisecond) {
				return true
			}
			return false
		}
		if s0 >= p.now() {
			break
		}
		if s := p.jb.pop(p.jb.nextSeq); s != nil {
			p.stats.LateDrop++
		}
		p.jb.advance()
	}
	if !p.jb.hasPending() {
		p.mu.Unlock()
		return false
	}
	dl, ok := p.deadlineLocal(p.jb.nextSeq)
	if !ok {
		p.mu.Unlock()
		return false
	}
	devLat := p.deviceLatencyNs
	p.mu.Unlock()

	// Start the pipeline at (deadline − deviceLatency): the first real frame, after it sits
	// the device buffer's worth in the queue, is heard at its deadline. No live queue probe.
	if d := (dl - devLat) - p.now(); d > 0 {
		if p.sleepInterruptible(time.Duration(d)) {
			return true
		}
	}
	// Pre-fill the device buffer with silence (~deviceLatency, min 2 frames) so the first
	// real frame plays AFTER that silence drains and the blocking write engages. The first
	// write also opens a lazily-opened (resilient) device.
	nPrefill := int(devLat / stream.FrameNanos)
	if nPrefill < 2 {
		nPrefill = 2
	}
	for i := 0; i < nPrefill; i++ {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return true
		}
		if !p.armed || p.gen != gen {
			p.mu.Unlock()
			return false
		}
		p.mu.Unlock()
		p.writeStartNs.Store(p.now())
		_ = p.out.Write(p.silence)
		p.writeStartNs.Store(0)
		select {
		case <-p.done:
			return true
		default:
		}
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return true
	}
	if !p.armed || p.gen != gen || !p.jb.hasPending() {
		p.mu.Unlock()
		return false
	}
	if p.latency != nil { // device is open now; refresh the constant offset for steady state
		if l := p.latency.ConfiguredLatencyNs(); l > 0 {
			p.deviceLatencyNs = l
		}
	}
	p.fedAnchorPTS = p.slotPTS(p.jb.nextSeq)
	p.fedAnchorCon = p.rs.consumedSamples() // measure fedPTS relative to NOW, not session start
	p.dacPrimed = true
	p.mu.Unlock()
	return false
}

// feedLocked tops up the resampler so it has at least needInput samples ahead of its
// cursor: it feeds real frames in seq order (counting Played), and silence for a gap
// inside the pending range (counting Silence). If it drains past the last received frame
// mid-top-up, the input has starved (a network gap exceeded the jitter buffer, or the
// stream ended) — it pads with silence so the resampler can still emit, counts it as
// Silence, and logs it (rate-limited) so the underrun is diagnosable (it was previously
// invisible). Caller holds mu and has ensured jb.hasPending() on entry.
func (p *Playout) feedLocked() {
	for p.rs.inputAvail() < needInput {
		if p.jb.hasPending() {
			if s := p.jb.pop(p.jb.nextSeq); s != nil {
				p.rs.feed(s.payload)
				p.stats.Played++
			} else {
				p.rs.feed(p.silence)
				p.stats.Silence++
			}
			p.jb.advance()
			continue
		}
		// Input starved: pad with silence (audible gap). Count + log (rate-limited).
		p.rs.feed(p.silence)
		p.stats.Silence++
		if t := p.now(); t-p.lastUnderrunLog > 1_000_000_000 {
			p.lastUnderrunLog = t
			p.log.Warn("playout underrun: input starved (jitter buffer empty)", "buffered", p.jb.len())
		}
	}
}

// observeLocked folds one post-write phase measurement into the servo and refreshes
// telemetry. Caller holds mu. A no-op for the ratio when the device has no phase
// probe (exec/file/null): the ratio stays 1 and the device's clock paces playout.
func (p *Playout) observeLocked() {
	now := p.now()
	// The live device queue and the configured latency are TELEMETRY / the fixed phase
	// offset — NOT control inputs. The loop regulates the play head (fedPTS) against the
	// master clock; the device buffer is a constant transport delay folded in below, not
	// a live feedback term (it is ~constant and only injects ±10 ms jitter). See PLAN.
	if p.delay != nil {
		if dd, ok := p.delay.Delay(); ok {
			p.stats.DeviceDelayNs = dd
		}
	}
	if p.latency != nil { // resilient opens lazily; keep the constant offset live
		if l := p.latency.ConfiguredLatencyNs(); l > 0 {
			p.deviceLatencyNs = l
		}
	}
	mShould, synced := p.clock.LocalToMaster(now)
	if synced {
		// phaseErr = (play head − constant device latency) − where the master says it
		// should be. >0 ⇒ ahead ⇒ servo slows (ratio<1). No live queue, no dead time.
		shouldPTS := mShould - p.bufferNs + p.delayOffsetNs - p.equalizeDelayNs
		fedPTS := p.fedAnchorPTS + int64((p.rs.consumedSamples()-p.fedAnchorCon)*1e9/float64(stream.SampleRate))
		phaseErr := fedPTS - p.deviceLatencyNs - shouldPTS
		p.servo.observe(phaseErr, now, synced)
	}
	p.stats.RatePPM = p.servo.ratePPM()
	p.stats.PhaseErrNs = p.servo.phaseErr()
	// Calibrated (D65): a synced clock + a real queue probe, so DeviceDelayNs−PhaseErrNs
	// is the stable per-room device-queue depth the master equalizes against.
	p.stats.Calibrated = synced && p.delay != nil

	if t := p.now(); t-p.lastServoLog > 1_000_000_000 {
		p.lastServoLog = t
		p.log.Debug("stats",
			"ratePPM", int64(p.stats.RatePPM),
			"phaseErrMs", p.stats.PhaseErrNs/1_000_000,
			"deviceDelayMs", p.stats.DeviceDelayNs/1_000_000,
			"devLatMs", p.deviceLatencyNs/1_000_000,
			"buffered", p.stats.Buffered,
			"played", p.stats.Played, "silence", p.stats.Silence, "late", p.stats.LateDrop)
	}
}

// watchWrites is the device-stall guard: if a blocking Write has been in flight far
// longer than the device buffer could justify, the device is wedged — interrupt it
// so the loop unparks and the starvation watchdog can fire RESTART. Without this a
// wedged device would hang the loop inside Write forever, invisibly.
func (p *Playout) watchWrites() {
	defer p.wg.Done()
	limit := 3 * p.deviceLatencyNs
	if limit < int64(500*time.Millisecond) {
		limit = int64(500 * time.Millisecond)
	}
	interval := time.Duration(limit / 2)
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			st := p.writeStartNs.Load()
			if st != 0 && p.now()-st > limit && p.interrupt != nil {
				p.log.Warn("device write stalled; interrupting", "stalledMs", (p.now()-st)/1_000_000)
				p.interrupt.Interrupt()
			}
		}
	}
}

// waitIdle blocks while disarmed/empty until a wake, done, or a watchdog tick.
// On the tick it returns so the loop re-runs its inline starvation check (§3.7).
// Returns true if Close was signaled.
func (p *Playout) waitIdle() (done bool) {
	timer := time.NewTimer(p.watchdog)
	defer timer.Stop()
	select {
	case <-p.wake:
		return false
	case <-p.done:
		return true
	case <-timer.C:
		return false
	}
}

// sleepInterruptible sleeps for d unless Close fires first. Returns true on Close.
func (p *Playout) sleepInterruptible(d time.Duration) (done bool) {
	if d <= 0 {
		select {
		case <-p.done:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-p.done:
		return true
	case <-timer.C:
		return false
	}
}

// checkStarvationLocked implements the 2 s watchdog (§3.7). The caller holds the
// mutex. It returns a non-nil func to invoke (after unlocking) when a RESTART must
// be fired; nil otherwise. On the second starvation interval it disarms.
func (p *Playout) checkStarvationLocked() (fireRestart func()) {
	if !p.armed || p.now()-p.lastPkt < int64(p.watchdog) {
		return nil
	}
	if !p.restartHit {
		p.restartHit = true
		// Advance lastPkt so the next watchdog interval is measured from now; the
		// second interval of continued silence triggers disarm.
		p.lastPkt = p.now()
		p.log.Warn("playout starved, requesting RESTART")
		restart := p.restart
		if restart == nil {
			return nil
		}
		return restart
	}
	// Second starvation interval after RESTART with no fresh frames: disarm.
	p.armed = false
	p.dacPrimed = false
	p.jb.reset()
	p.servo.reset()
	p.rs.reset()
	p.stats.Buffered = 0
	if p.flush != nil {
		p.flush.Flush()
	}
	p.log.Warn("playout still starved after RESTART, disarming")
	return nil
}
