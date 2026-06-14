package sink

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ensemble/internal/stream"
)

// ENGINE integration tests for the DAC-pull, phase-locked Playout (sink.go).
//
// The control-loop deadline scheduler is GONE: the device's blocking Write is the
// only rate pacer (PLAN-dac-pull-phase-lock). So the old deadline late-drop tests
// (TestPlayoutLateFrameDropped, TestPlayoutBufferMsLead exact-deadline timing) no
// longer apply — there is no per-frame deadline sleep to miss. Prime's overdue-frame
// skip still late-drops, but steady state never does. We therefore prefer
// convergence/threshold assertions over exact wall-clock counts.
//
// To stay deterministic AND fast, the fake DAC drives a VIRTUAL clock: each Write
// (one frame handed to the device) advances simulated time by FrameNanos and sleeps
// a tiny REAL duration so the scheduler goroutine actually yields (the loop is paced,
// the test runs in milliseconds). Both the injected now() and the fake master/local
// clock read this same virtual clock, so phase error is a function of frames-written,
// not of real scheduling jitter.

// ---- virtual clock ---------------------------------------------------------

// vclock is simulated monotonic time in ns, advanced by the DAC as it consumes
// frames. Shared (atomically) by the DAC fake, the fakeClock and the injected now.
type vclock struct{ ns atomic.Int64 }

func (v *vclock) now() int64  { return v.ns.Load() }
func (v *vclock) add(d int64) { v.ns.Add(d) }
func newVClock() *vclock      { v := &vclock{}; v.ns.Store(1_000_000_000); return v } // start at t=1s

// ---- fake clock (master == local, toggleable sync) -------------------------

type fakeClock struct {
	v      *vclock
	synced atomic.Bool
}

func newFakeClock(v *vclock, synced bool) *fakeClock {
	c := &fakeClock{v: v}
	c.synced.Store(synced)
	return c
}
func (c *fakeClock) setSynced(s bool)                    { c.synced.Store(s) }
func (c *fakeClock) MasterNow() (int64, bool)            { return c.v.now(), c.synced.Load() }
func (c *fakeClock) MasterToLocal(m int64) (int64, bool) { return m, c.synced.Load() }
func (c *fakeClock) LocalToMaster(l int64) (int64, bool) { return l, c.synced.Load() }

// ---- virtual DAC -----------------------------------------------------------

// virtualDAC models a real sound card: a blocking Write (the rate pacer) and a
// device queue draining at a crystal skewed dacPPM off nominal. It implements
// device.Sink + DelayReporter + LatencyReporter + Interrupter.
//
// QUEUE MODEL (all in virtual ns):
//   - Write adds FrameNanos of audio to the queue, advances the virtual clock by
//     FrameNanos, and sleeps `pace` of REAL time so the loop goroutine yields.
//   - The DAC continuously drains at (1+dacPPM/1e6) ×real-time. We compute the
//     drained amount lazily from the virtual clock: drained(t) = (t−t0)*(1+ppm).
//   - Delay() returns queue = written − drained, floored at 0 (a real card cannot
//     report negative latency; flooring models an underrun). It starts ok=false
//     until the first write so prime's "no signal yet" branch is exercised.
//
// A fast DAC (ppm>0) drains faster than frames arrive, so to hold the queue the
// engine must SLOW production (ratio<1 ⇒ RatePPM<0): RatePPM → −dacPPM at lock.
type virtualDAC struct {
	mu          sync.Mutex
	v           *vclock
	dacPPM      float64
	pace        time.Duration // real sleep per Write (loop pacing)
	t0          int64         // virtual ns of the first write (drain origin)
	writtenNs   float64       // cumulative audio handed to the device (ns)
	started     bool
	configLatNs int64
	interrupted atomic.Bool
	closed      atomic.Bool
}

func newVirtualDAC(v *vclock, dacPPM float64, pace time.Duration) *virtualDAC {
	const configLat = 200 * 1_000_000 // 200 ms configured buffer
	return &virtualDAC{
		v:           v,
		dacPPM:      dacPPM,
		pace:        pace,
		configLatNs: configLat,
		// Seed the buffer cushion as already pre-filled so the queue holds ~configLat in
		// steady state (the blocking write maintains it there); see Write.
		writtenNs: configLat,
	}
}

// drainedLocked returns audio drained since t0 at the skewed DAC rate.
func (d *virtualDAC) drainedLocked() float64 {
	if !d.started {
		return 0
	}
	dt := float64(d.v.now() - d.t0)
	if dt < 0 {
		dt = 0
	}
	return dt * (1 + d.dacPPM/1e6)
}

func (d *virtualDAC) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return errBadFrame
	}
	if d.closed.Load() {
		return nil
	}
	d.mu.Lock()
	if !d.started {
		d.started = true
		d.t0 = d.v.now()
	}
	d.writtenNs += float64(stream.FrameNanos)
	d.mu.Unlock()
	// Advance virtual time by the REAL time the blocking write waited: the DAC must drain
	// one frame's worth of room, which at a crystal skewed dacPPM takes FrameNanos/(1+ppm)
	// (a fast DAC frees room sooner). This is what couples the drift into the engine's
	// phase error (fedPTS advances at the input rate, shouldPTS at this DAC-paced wall
	// clock), so the constant-latency servo locks to ratePPM → −dacPPM. Then yield real
	// time so the loop goroutine is paced.
	d.v.add(int64(float64(stream.FrameNanos) / (1 + d.dacPPM/1e6)))
	if d.pace > 0 && !d.interrupted.Load() {
		time.Sleep(d.pace)
	}
	return nil
}

func (d *virtualDAC) Close() error { d.closed.Store(true); return nil }

// Delay reports the live queue depth in ns (the servo's phase probe).
func (d *virtualDAC) Delay() (int64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return 0, false
	}
	q := d.writtenNs - d.drainedLocked()
	if q < 0 {
		q = 0 // underrun: a real card floors at zero
	}
	return int64(q), true
}

func (d *virtualDAC) ConfiguredLatencyNs() int64 { return d.configLatNs }

func (d *virtualDAC) Interrupt() { d.interrupted.Store(true) }

// errBadFrame is returned for a wrong-size Write (the package contract).
var errBadFrame = errBadFrameT("frame size")

type errBadFrameT string

func (e errBadFrameT) Error() string { return string(e) }

// recBackend is a NON-paced recorder for lifecycle tests that only need to count
// frames / inspect content (gap, reorder, gain, stale gen). It blocks just enough
// to yield (the rate pacer) and advances the virtual clock so the watchdog/now
// arithmetic stays consistent. No queue model → no phase probe (the servo holds
// ratio 1, the device's clock would pace in production).
type recBackend struct {
	mu     sync.Mutex
	v      *vclock
	frames [][]byte
	pace   time.Duration
}

func newRecBackend(v *vclock) *recBackend { return &recBackend{v: v, pace: 150 * time.Microsecond} }

func (b *recBackend) Write(f []byte) error {
	if len(f) != stream.FrameBytes {
		return errBadFrame
	}
	b.mu.Lock()
	cp := make([]byte, len(f))
	copy(cp, f)
	b.frames = append(b.frames, cp)
	b.mu.Unlock()
	b.v.add(stream.FrameNanos)
	if b.pace > 0 {
		time.Sleep(b.pace)
	}
	return nil
}
func (b *recBackend) Close() error { return nil }
func (b *recBackend) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.frames)
}
func (b *recBackend) at(i int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.frames[i]
}

// ---- helpers ---------------------------------------------------------------

// audioFrame builds a frame whose every sample equals v.
func audioFrame(v int16) []byte {
	f := make([]byte, stream.FrameBytes)
	for i := 0; i < stream.FrameSamples*stream.Channels; i++ {
		binary.LittleEndian.PutUint16(f[i*2:i*2+2], uint16(v))
	}
	return f
}

// drainUntil polls cond up to ~5 s of real time, yielding to the scheduler.
func drainUntil(cond func() bool) bool {
	for i := 0; i < 50000; i++ {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Microsecond)
	}
	return cond()
}

// pushRun pushes seqs with pts anchored a little ahead of the current virtual
// master time so their (now-gone) deadlines and the prime alignment land just
// ahead — never overdue at prime, so nothing is skipped.
func pushRun(p *Playout, v *vclock, gen uint32, seqs []uint64) {
	base := v.now() + int64(5*time.Millisecond)
	for _, s := range seqs {
		pts := base + int64(s)*stream.FrameNanos
		p.Push(gen, s, pts, audioFrame(int16(1000+s)))
	}
}

// feedFor keeps the sink fed DEMAND-DRIVEN: it tops the jitter buffer up toward a
// small target depth and otherwise idles. Decoupling the push rate from real time
// means the DAC's blocking Write alone paces the loop (the design contract), so the
// buffer stays bounded no matter how fast the test's feeder goroutine runs — the
// engine cannot be force-fed into a runaway. Runs until stop is closed; used by the
// convergence test, which must run long enough for the servo to lock.
func feedFor(p *Playout, v *vclock, gen uint32, stop <-chan struct{}) {
	go func() {
		seq := uint64(0)
		base := v.now() + int64(40*time.Millisecond)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if p.Stats().Buffered < 12 {
				p.Push(gen, seq, base+int64(seq)*stream.FrameNanos, audioFrame(500))
				seq++
			} else {
				time.Sleep(20 * time.Microsecond)
			}
		}
	}()
}

// engineServoCfg is a hot LP-P tune for the live-engine convergence tests: a brisk gain
// and short filter so the loop locks within a few seconds of paced real time. Hotter than
// production defaults (which low-pass the Pi's snd_pcm_delay jitter); the virtual DAC is
// noise-free so it can afford it. At Kp 0.3 the P-only standing error for the test's
// 150 ppm DAC is dacPPM/(1e6·Kp) ≈ 500 µs.
func engineServoCfg() servoConfig {
	return servoConfig{Kp: 0.3, N: 4, ClampPPM: 300, SlewPPM: 50000}
}

// newTestPlayout wires a Playout against the virtual clock + a backend, fast
// watchdog off by default (long), fast servo gains.
func newTestPlayout(t *testing.T, v *vclock, clk *fakeClock, be interface {
	Write([]byte) error
	Close() error
}, restart RestartFunc) *Playout {
	t.Helper()
	p := New(Config{
		Backend:  be,
		Clock:    clk,
		BufferMs: 150,
		Restart:  restart,
		Volume:   1.0,
		Watchdog: 30 * time.Second, // dedicated watchdog tests set it short
		now:      v.now,
		servoCfg: fastServoCfg(),
	})
	t.Cleanup(func() { p.Close() })
	return p
}

// ---- THE KEY TEST: the loop closes ----------------------------------------

// TestPlayoutServoLocksToDAC validates the closed loop end-to-end: with the virtual DAC
// running +150 ppm fast, after enough paced frames the servo LOCKS — RatePPM tracks ≈ −150
// (slow production to match the fast drain), PhaseErrNs settles at the small P-only
// standing error (≈ 500 µs at Kp 0.3, well inside the ±3 ms bound), and the jitter buffer
// stays bounded (no runaway). If this does NOT lock it is a real engine bug, reported
// loudly rather than hidden.
func TestPlayoutServoLocksToDAC(t *testing.T) {
	const dacPPM = 150.0
	v := newVClock()
	clk := newFakeClock(v, true)
	dac := newVirtualDAC(v, dacPPM, 10*time.Microsecond)
	p := New(Config{
		Backend:  dac,
		Clock:    clk,
		BufferMs: 20,  // small playout lead ⇒ prime hands over near phase 0, so the
		Volume:   1.0, // integral has only the rate offset to absorb (fast lock)
		Watchdog: 30 * time.Second,
		now:      v.now,
		servoCfg: engineServoCfg(),
	})
	t.Cleanup(func() { p.Close() })
	p.Reset(1)

	stop := make(chan struct{})
	defer close(stop)
	feedFor(p, v, 1, stop)

	// Wait for LOCK: RatePPM tracks −dacPPM (±20) AND |PhaseErrNs| under ~3 ms. The
	// rate match (RatePPM → −150) is the proof the loop closed — production slows to
	// exactly cancel the fast drain; P-only then holds a small standing phase offset.
	locked := drainUntil(func() bool {
		s := p.Stats()
		return s.RatePPM < -130 && s.RatePPM > -170 &&
			s.PhaseErrNs < 3_000_000 && s.PhaseErrNs > -3_000_000
	})
	s := p.Stats()
	if !locked {
		// Do NOT weaken this to hide a non-converging loop — that is a real engine bug.
		t.Fatalf("ENGINE LOOP DID NOT CLOSE: RatePPM=%.1f (want ≈ -150), PhaseErrNs=%d (want ≈0), Buffered=%d",
			s.RatePPM, s.PhaseErrNs, s.Buffered)
	}
	if s.Buffered > defaultCapacity {
		t.Fatalf("jitter buffer unbounded under lock: Buffered=%d", s.Buffered)
	}
	t.Logf("locked: RatePPM=%.1f (want ≈ -150), PhaseErrNs=%d ns, Buffered=%d", s.RatePPM, s.PhaseErrNs, s.Buffered)
}

// TestPlayoutRatePPMClamps: an absurd DAC skew saturates the servo at the clamp,
// never beyond, and the buffer still stays bounded.
func TestPlayoutRatePPMClamps(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	dac := newVirtualDAC(v, 20000, 10*time.Microsecond) // absurd +20000 ppm
	p := New(Config{
		Backend: dac, Clock: clk, BufferMs: 20, Volume: 1,
		Watchdog: 30 * time.Second, now: v.now, servoCfg: engineServoCfg(),
	})
	t.Cleanup(func() { p.Close() })
	p.Reset(1)
	stop := make(chan struct{})
	defer close(stop)
	feedFor(p, v, 1, stop)
	clampPPM := engineServoCfg().ClampPPM
	drainUntil(func() bool { return p.Stats().RatePPM <= -(clampPPM - 1) })
	ppm := p.Stats().RatePPM
	if ppm < -clampPPM-0.001 || ppm > clampPPM+0.001 {
		t.Fatalf("RatePPM %.1f outside ±%.0f clamp", ppm, clampPPM)
	}
}

// ---- lifecycle (ported, adapted to the paced engine) -----------------------

func TestPlayoutBasicInOrder(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	if !drainUntil(func() bool { return p.Stats().Played >= 10 }) {
		t.Fatalf("Played=%d, want >=10", p.Stats().Played)
	}
	if s := p.Stats(); s.Silence != 0 {
		t.Fatalf("Silence=%d, want 0 for an in-order run", s.Silence)
	}
}

func TestPlayoutInsertsSilenceForGap(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	// Miss seq 2: the engine fills one silence frame for the gap, plays the rest.
	pushRun(p, v, 1, []uint64{0, 1, 3, 4})
	if !drainUntil(func() bool { return p.Stats().Played >= 4 && p.Stats().Silence >= 1 }) {
		s := p.Stats()
		t.Fatalf("gap not filled: Played=%d Silence=%d, want Played>=4 Silence>=1", s.Played, s.Silence)
	}
}

func TestPlayoutReorderWithinBuffer(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	// Push 0,2,1,3 quickly: the jitter buffer reorders so all four play, no gap.
	p.Push(1, 0, v.now()+int64(5*time.Millisecond), audioFrame(10))
	p.Push(1, 2, v.now()+int64(5*time.Millisecond)+2*stream.FrameNanos, audioFrame(12))
	p.Push(1, 1, v.now()+int64(5*time.Millisecond)+1*stream.FrameNanos, audioFrame(11))
	p.Push(1, 3, v.now()+int64(5*time.Millisecond)+3*stream.FrameNanos, audioFrame(13))
	if !drainUntil(func() bool { return p.Stats().Played >= 4 }) {
		s := p.Stats()
		t.Fatalf("reorder not played in order: Played=%d Silence=%d", s.Played, s.Silence)
	}
	if s := p.Stats(); s.Silence != 0 {
		t.Fatalf("reorder produced %d silence frames, want 0", s.Silence)
	}
}

func TestPlayoutStaleGenDropped(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(2)
	p.Push(1, 0, v.now(), audioFrame(1)) // gen 1 is stale (armed for gen 2)
	if !drainUntil(func() bool { return p.Stats().StaleGen >= 1 }) {
		t.Fatalf("StaleGen=%d, want >=1", p.Stats().StaleGen)
	}
	if be.count() != 0 {
		t.Fatalf("stale frame written, count=%d", be.count())
	}
}

func TestPlayoutUnsyncedHoldsThenFlushes(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, false) // unsynced
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1, 2})
	time.Sleep(20 * time.Millisecond)
	if be.count() != 0 {
		t.Fatalf("unsynced: wrote %d frames, want 0 (the §7 gate holds)", be.count())
	}
	if p.Stats().Synced {
		t.Fatal("Synced should be false while the fake clock is unsynced")
	}
	clk.setSynced(true)
	// Fresh near-future frames now flush.
	pushRun(p, v, 1, []uint64{3, 4, 5})
	if !drainUntil(func() bool { return be.count() >= 1 }) {
		t.Fatal("frames should flush after sync")
	}
	if !p.Stats().Synced {
		t.Fatal("Synced should be true after sync")
	}
}

func TestPlayoutBufferStat(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, false) // unsynced so frames accumulate, none drain
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	for s := uint64(0); s < 5; s++ {
		p.Push(1, s, v.now()+int64(s)*stream.FrameNanos, audioFrame(1))
	}
	if !drainUntil(func() bool { return p.Stats().Buffered == 5 }) {
		t.Fatalf("Buffered=%d, want 5", p.Stats().Buffered)
	}
}

func TestPlayoutNoLatencyReporterIsZero(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	p := newTestPlayout(t, v, clk, newRecBackend(v), nil)
	if p.deviceLatencyNs != 0 {
		t.Fatalf("deviceLatencyNs=%d, want 0 for a backend without LatencyReporter", p.deviceLatencyNs)
	}
}

func TestPlayoutLatencyReporterRead(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	dac := newVirtualDAC(v, 0, 150*time.Microsecond)
	p := New(Config{Backend: dac, Clock: clk, BufferMs: 150, Volume: 1, now: v.now, servoCfg: fastServoCfg()})
	t.Cleanup(func() { p.Close() })
	if p.deviceLatencyNs != dac.ConfiguredLatencyNs() {
		t.Fatalf("deviceLatencyNs=%d, want %d from the DAC's LatencyReporter", p.deviceLatencyNs, dac.ConfiguredLatencyNs())
	}
}

func TestPlayoutSetGainHalves(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	p.SetGain(0.5)
	const val = 8000
	base := v.now() + int64(5*time.Millisecond)
	for s := uint64(0); s < 10; s++ {
		p.Push(1, s, base+int64(s)*stream.FrameNanos, audioFrame(val))
	}
	if !drainUntil(func() bool { return p.Stats().Played >= 6 }) {
		t.Fatalf("frames not played: Played=%d", p.Stats().Played)
	}
	// The gain ramp settles within a frame; a late written frame is fully halved.
	last := be.at(be.count() - 1)
	mid := int16(binary.LittleEndian.Uint16(last[100:102]))
	if mid < val/2-80 || mid > val/2+80 {
		t.Fatalf("settled gain not ~0.5: sample=%d want ~%d", mid, val/2)
	}
}

func TestPlayoutResetZeroesCounters(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1})
	drainUntil(func() bool { return p.Stats().Played >= 1 })
	p.Reset(2)
	st := p.Stats()
	if st.Played != 0 || st.Silence != 0 || st.LateDrop != 0 || st.StaleGen != 0 || st.RatePPM != 0 || st.PhaseErrNs != 0 {
		t.Fatalf("Reset did not zero counters: %+v", st)
	}
}

func TestPlayoutCloseIdempotentNoLeak(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1, 2})
	time.Sleep(10 * time.Millisecond)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil { // idempotent
		t.Fatalf("second Close: %v", err)
	}
	// Push/Reset after Close must not panic.
	p.Push(1, 9, v.now(), audioFrame(1))
	p.Reset(3)
}

// TestPlayoutCloseUnblocksParkedWrite: Close must interrupt a Write parked inside a
// slow device so wg.Wait cannot hang (the Interrupter path).
func TestPlayoutCloseUnblocksParkedWrite(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	// A DAC whose Write sleeps a long real time models a wedged device; Interrupt
	// flips the flag so the sleep is skipped and Close's wg.Wait returns promptly.
	dac := newVirtualDAC(v, 0, 200*time.Millisecond)
	p := New(Config{Backend: dac, Clock: clk, BufferMs: 150, Volume: 1, now: v.now, servoCfg: fastServoCfg()})
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1, 2})
	time.Sleep(10 * time.Millisecond) // let the loop park inside a slow Write
	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung on a parked Write (Interrupter not honored)")
	}
}

// The watchdog measures p.now()−lastPkt and only wakes the idle loop on a real
// time.Timer of p.watchdog. The VIRTUAL clock freezes when idle (it only advances
// on Writes), so the watchdog would never age. These two tests therefore wire a
// WALL-clock now() + a wall-backed master/local clock: time genuinely passes while
// idle, the watchdog ages naturally, and a short Watchdog keeps the test fast. The
// loop body (feed/render/observe) is identical; only the clock source differs.

// wallClock is master==local==wall-time-since-base, gated by synced.
type wallClock struct {
	base   time.Time
	synced atomic.Bool
}

func newWallClock(synced bool) *wallClock {
	c := &wallClock{base: time.Now()}
	c.synced.Store(synced)
	return c
}
func (c *wallClock) now() int64                          { return int64(time.Since(c.base)) }
func (c *wallClock) MasterNow() (int64, bool)            { return c.now(), c.synced.Load() }
func (c *wallClock) MasterToLocal(m int64) (int64, bool) { return m, c.synced.Load() }
func (c *wallClock) LocalToMaster(l int64) (int64, bool) { return l, c.synced.Load() }

// wallBackend records frames, paced by a tiny real sleep; no virtual clock.
type wallBackend struct {
	mu     sync.Mutex
	frames int
}

func (b *wallBackend) Write(f []byte) error {
	if len(f) != stream.FrameBytes {
		return errBadFrame
	}
	b.mu.Lock()
	b.frames++
	b.mu.Unlock()
	time.Sleep(150 * time.Microsecond)
	return nil
}
func (b *wallBackend) Close() error { return nil }
func (b *wallBackend) count() int   { b.mu.Lock(); defer b.mu.Unlock(); return b.frames }

// pushRunWall pushes seqs with pts just ahead of the wall master clock.
func pushRunWall(p *Playout, clk *wallClock, gen uint32, seqs []uint64) {
	base := clk.now() + int64(5*time.Millisecond)
	for _, s := range seqs {
		p.Push(gen, s, base+int64(s)*stream.FrameNanos, audioFrame(int16(1000+s)))
	}
}

// TestPlayoutWatchdogFiresRestartThenDisarms drives the starvation watchdog off
// the VIRTUAL clock: starvation is detected from now()-lastPkt, so advancing the
// vclock by hand is what crosses each watchdog interval. That makes the whole
// sequence deterministic — in particular the "stays armed, then resume" step can
// never race the second (disarm) interval, because virtual time only moves when
// this test moves it. (An earlier wall-clock version flaked under CI load when the
// resume push missed the real 60 ms window before disarm.) The real timer in
// waitIdle only governs how soon the idle loop re-checks; drainUntil's budget
// absorbs that. Watchdog is well above the ~60 ms of virtual time the initial
// prime+play advances, so nothing auto-fires before the explicit add.
func TestPlayoutWatchdogFiresRestartThenDisarms(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	var mu sync.Mutex
	var restarts int
	restart := func() { mu.Lock(); restarts++; mu.Unlock() }
	getR := func() int { mu.Lock(); defer mu.Unlock(); return restarts }
	const wd = 100 * time.Millisecond
	p := New(Config{
		Backend: be, Clock: clk, BufferMs: 150, Restart: restart, Volume: 1,
		Watchdog: wd, now: v.now, servoCfg: fastServoCfg(),
	})
	t.Cleanup(func() { p.Close() })
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0})
	if !drainUntil(func() bool { return p.Stats().Played >= 1 }) {
		t.Fatal("seq 0 did not play")
	}

	// One watchdog interval of silence → RESTART fires once.
	v.add(int64(2 * wd))
	if !drainUntil(func() bool { return getR() >= 1 }) {
		t.Fatalf("watchdog did not fire RESTART: restarts=%d", getR())
	}
	// Sink stays armed after RESTART: a resumed push still plays. Virtual time is
	// frozen here, so the disarm interval cannot race this.
	before := be.count()
	pushRun(p, v, 1, []uint64{1})
	if !drainUntil(func() bool { return be.count() > before }) {
		t.Fatal("sink did not resume after RESTART")
	}
	if getR() != 1 {
		t.Fatalf("RESTART should fire once per armed session, got %d", getR())
	}
	// A second sustained-silence interval after the RESTART → disarm.
	v.add(int64(2 * wd))
	if !drainUntil(func() bool { _, armed := p.ArmedGen(); return !armed }) {
		t.Fatal("watchdog did not disarm after a second starved interval")
	}
	// Re-arm via Reset resumes playout.
	atDisarm := be.count()
	p.Reset(2)
	pushRun(p, v, 2, []uint64{0, 1})
	if !drainUntil(func() bool { return be.count() > atDisarm }) {
		t.Fatal("sink did not re-arm after Reset")
	}
}

func TestPlayoutWatchdogNilRestart(t *testing.T) {
	clk := newWallClock(true)
	be := &wallBackend{}
	p := New(Config{
		Backend: be, Clock: clk, BufferMs: 150, Restart: nil, Volume: 1,
		Watchdog: 50 * time.Millisecond, now: clk.now, servoCfg: fastServoCfg(),
	})
	t.Cleanup(func() { p.Close() })
	p.Reset(1)
	pushRunWall(p, clk, 1, []uint64{0})
	drainUntil(func() bool { return p.Stats().Played >= 1 })
	// Two intervals of silence: no panic with a nil restart hook, then disarms.
	if !drainUntil(func() bool { _, armed := p.ArmedGen(); return !armed }) {
		t.Fatal("nil-restart watchdog did not disarm after sustained silence")
	}
	// Re-arm works.
	p.Reset(2)
	pushRunWall(p, clk, 2, []uint64{0})
	if !drainUntil(func() bool { return be.count() >= 1 }) {
		t.Fatal("did not resume after nil-restart disarm + Reset")
	}
}

// TestPlayoutSetDelayOffsetReanchors: SetDelayOffset re-anchors (discards the
// buffer, fires restart once, re-primes on the next Push). The deadline scheduler
// is gone, so we assert the re-anchor SIDE EFFECTS, not an exact play time.
func TestPlayoutSetDelayOffsetReanchors(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	var mu sync.Mutex
	var restarts int
	restart := func() { mu.Lock(); restarts++; mu.Unlock() }
	p := newTestPlayout(t, v, clk, be, restart)
	p.Reset(1)
	pushRun(p, v, 1, []uint64{0, 1, 2, 3, 4})
	drainUntil(func() bool { return p.Stats().Played >= 3 })

	mu.Lock()
	restarts = 0
	mu.Unlock()
	p.SetDelayOffset(int64(50 * time.Millisecond))
	mu.Lock()
	r := restarts
	mu.Unlock()
	if r != 1 {
		t.Fatalf("SetDelayOffset should fire restart exactly once, got %d", r)
	}
	if p.Stats().Buffered != 0 {
		t.Fatalf("buffer not discarded on delay change: %d", p.Stats().Buffered)
	}
	if p.delayOffsetNs != int64(50*time.Millisecond) {
		t.Fatalf("delayOffsetNs=%d, want %d", p.delayOffsetNs, int64(50*time.Millisecond))
	}
	// Re-primes on the next Push.
	before := be.count()
	pushRun(p, v, 1, []uint64{10, 11, 12})
	if !drainUntil(func() bool { return be.count() > before }) {
		t.Fatal("did not re-prime after the delay-offset re-anchor")
	}
}

func TestPlayoutSetDelayOffsetClampsAndNilRestart(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil) // nil restart: must not panic
	p.Reset(1)
	// A wildly negative offset clamps to −maxDelayMs (D36, §1).
	p.SetDelayOffset(int64(-10 * time.Second))
	if p.delayOffsetNs != -int64(maxDelayMs)*1_000_000 {
		t.Fatalf("negative offset not clamped: %d, want %d", p.delayOffsetNs, -int64(maxDelayMs)*1_000_000)
	}
	if p.Stats().Buffered != 0 {
		t.Fatalf("buffer not discarded: %d", p.Stats().Buffered)
	}
}

// TestPlayoutSetEqualizeDelay (D65): re-anchors on a real change, dedups an
// unchanged re-assert (the master re-asserts every heartbeat; re-anchoring each
// time would be audible), and clamps negatives to zero (it only ever delays).
func TestPlayoutSetEqualizeDelay(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	var mu sync.Mutex
	var restarts int
	restart := func() { mu.Lock(); restarts++; mu.Unlock() }
	p := newTestPlayout(t, v, clk, be, restart)
	p.Reset(1)
	get := func() int { mu.Lock(); defer mu.Unlock(); return restarts }

	p.SetEqualizeDelay(int64(70 * time.Millisecond)) // change → re-anchor
	if got := get(); got != 1 {
		t.Fatalf("restarts after first set = %d, want 1", got)
	}
	p.SetEqualizeDelay(int64(70 * time.Millisecond)) // unchanged → dedup
	if got := get(); got != 1 {
		t.Fatalf("restarts after identical re-assert = %d, want 1 (must dedup)", got)
	}
	p.SetEqualizeDelay(-5_000_000) // negative clamps to 0; 70 → 0 is a change → re-anchor
	if got := get(); got != 2 {
		t.Fatalf("restarts after clamp-to-zero = %d, want 2", got)
	}
	if p.equalizeDelayNs != 0 {
		t.Fatalf("equalizeDelayNs=%d, want 0 after the negative clamp", p.equalizeDelayNs)
	}
	p.SetEqualizeDelay(0) // already 0 → dedup
	if got := get(); got != 2 {
		t.Fatalf("restarts after redundant zero = %d, want 2", got)
	}
}

// TestPlayoutStatsConcurrent: Stats/SetGain hammered from another goroutine while
// the loop runs must be race-free (run under -race).
func TestPlayoutStatsConcurrent(t *testing.T) {
	v := newVClock()
	clk := newFakeClock(v, true)
	be := newRecBackend(v)
	p := newTestPlayout(t, v, clk, be, nil)
	p.Reset(1)
	stop := make(chan struct{})
	go func() {
		seq := uint64(0)
		for {
			select {
			case <-stop:
				return
			default:
			}
			p.Push(1, seq, v.now()+int64(5*time.Millisecond)+int64(seq)*stream.FrameNanos, audioFrame(500))
			seq++
			time.Sleep(200 * time.Microsecond)
		}
	}()
	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case <-deadline:
			close(stop)
			return
		default:
			_ = p.Stats()
			p.SetGain(0.8)
		}
	}
}
