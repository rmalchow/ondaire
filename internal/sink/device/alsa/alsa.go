// Package alsa is the ALSA output adapter for the device port. It writes canonical
// PCM (48 kHz stereo s16le, 20 ms frames) straight to an ALSA PCM device via
// runtime-loaded libasound (no cgo; D32/D34). It implements device.Sink plus the
// optional DelayReporter / LatencyReporter / Flusher / Interrupter / StatsReporter
// capabilities the servo and the resilient failover backend probe for with
// device.Query.
//
// The libasound probe runs in init(): on success the adapter registers the "alsa"
// backend (factory + enumerator + candidate provider); on failure it registers
// nothing, so a host without libasound simply has no "alsa" kind (D3) — never a
// panic, never a build tag.
//
// LOCKING MODEL — the whole reason this adapter exists as its own port. The
// blocking syscall is snd_pcm_writei, and Interrupt()/Flush() (snd_pcm_drop) must be
// able to abort a write that is parked waiting for buffer space, possibly from
// ANOTHER goroutine. So Write does NOT hold the mutex across writei:
//
//   - The mutex guards only the small mutable state (the pcm handle, the closed
//     flag, the xrun counters/log timestamp). It is held for brief critical
//     sections, never across a blocking libasound call.
//   - Write takes the mutex just long enough to snapshot {pcm, closed}, then runs
//     writei UNLOCKED. On an xrun it re-takes the mutex to bump counters / log and to
//     run the recover path, then drops it again for the retry writei.
//   - Interrupt()/Flush() take the mutex only to snapshot {pcm, closed} and then call
//     snd_pcm_drop UNLOCKED. drop is the documented way to unstick a blocked writei,
//     which then returns an error and the engine unwinds.
//   - libasound's per-PCM calls are individually thread-safe, so concurrent
//     writei + drop on the same handle is sound; the mutex exists only to protect OUR
//     Go state and to fence handle use against Close.
//   - Close sets closed under the mutex and calls snd_pcm_close. The engine
//     guarantees Interrupt-before-Close-before-the-writer-exits, so no writei is in
//     flight when Close runs; the closed flag merely makes Close idempotent and stops
//     any later Write/Delay/Flush from touching a closed handle.
package alsa

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"ensemble/internal/dl"
	"ensemble/internal/sink/device"
	"ensemble/internal/stream"
)

// ALSA simple-API constants (D34).
const (
	sndPCMStreamPlayback   = 0  // SND_PCM_STREAM_PLAYBACK
	sndPCMFormatS16LE      = 2  // SND_PCM_FORMAT_S16_LE
	sndPCMAccessRWInterlvd = 3  // SND_PCM_ACCESS_RW_INTERLEAVED
	alsaSoftResample       = 1  // enable libasound's own resampler if needed
	defaultLatencyMs       = 40 // ~2 frames: scheduling slack + something for the blocking
	//                              write to block on. NOT the jitter budget — that lives in
	//                              the INPUT jitter buffer (the device buffer can't absorb
	//                              network jitter). Tunable up via ENSEMBLE_ALSA_LATENCY_MS.
	minLatencyMs = 20
	maxLatencyMs = 500
)

// alsaSonames / alsaSymbols feed the dl.Open probe (D32/D34).
var (
	alsaSonames = []string{"libasound.so.2", "libasound.so"}
	alsaSymbols = []string{
		"snd_pcm_open", "snd_pcm_set_params", "snd_pcm_writei",
		"snd_pcm_recover", "snd_pcm_delay", "snd_pcm_close",
		"snd_pcm_drop", "snd_pcm_prepare",
	}
)

// funcs holds the bound libasound symbols, bound once at init().
type funcs struct {
	lib       *dl.Lib
	open      func(pcmp *uintptr, name string, stream, mode int32) int32
	setParams func(pcm uintptr, format, access, channels, rate, softResample, latencyUs int32) int32
	writei    func(pcm uintptr, buf uintptr, frames uintptr) int
	recover   func(pcm uintptr, err, silent int32) int32
	delay     func(pcm uintptr, delayp *int) int32
	close     func(pcm uintptr) int32
	drop      func(pcm uintptr) int32
	prepare   func(pcm uintptr) int32
}

// bound is the bound symbol table, non-nil only when the init() probe succeeded.
var bound *funcs

// init probes libasound and, on success, registers the "alsa" backend, its
// enumerator and its candidate provider. On probe failure it registers nothing
// (capability off; the kind is simply absent — D3), exactly like the old backend.
func init() {
	f, err := probe()
	if err != nil {
		return // capability off; "alsa" not registered
	}
	bound = f

	device.Register("alsa", func(arg string, log *slog.Logger) (device.Sink, error) {
		// arg carries the configured device (D37); empty => "default".
		return newSink(arg, log)
	}, func() bool { return bound != nil })

	device.RegisterEnumerator("alsa", ListOutputDevices)
	device.RegisterCandidates("alsa", candidates)
}

// probe runs the dl.Open capability probe and binds every symbol. Open has already
// verified each symbol resolves, so the Func binds below never fail.
func probe() (*funcs, error) {
	lib, err := dl.Open(alsaSonames, alsaSymbols)
	if err != nil {
		return nil, err
	}
	f := &funcs{lib: lib}
	lib.Func(&f.open, "snd_pcm_open")
	lib.Func(&f.setParams, "snd_pcm_set_params")
	lib.Func(&f.writei, "snd_pcm_writei")
	lib.Func(&f.recover, "snd_pcm_recover")
	lib.Func(&f.delay, "snd_pcm_delay")
	lib.Func(&f.drop, "snd_pcm_drop")
	lib.Func(&f.prepare, "snd_pcm_prepare")
	lib.Func(&f.close, "snd_pcm_close")
	return f, nil
}

// sink is one open ALSA PCM device. See the package LOCKING MODEL comment for the
// invariants on mu, pcm and closed.
type sink struct {
	f         *funcs
	latencyMs int // configured device latency (snd_pcm_set_params); D63 pre-roll
	log       *slog.Logger

	mu          sync.Mutex // guards the fields below; NEVER held across writei/drop
	pcm         uintptr    // libasound PCM handle
	closed      bool       // Close ran; handle must not be used again
	framesWr    uint64     // frames accepted by the device (telemetry)
	writeErrs   uint64     // write failures observed (telemetry)
	xruns       uint64     // device underruns recovered (telemetry == Underruns)
	lastDelayNs int64      // most recent successful Delay() reading (telemetry)
	lastXrunLog time.Time  // rate-limit for the xrun warning (1/s)
}

// compile-time assertions: sink honours Sink and every advertised capability.
var (
	_ device.Sink            = (*sink)(nil)
	_ device.DelayReporter   = (*sink)(nil)
	_ device.LatencyReporter = (*sink)(nil)
	_ device.Flusher         = (*sink)(nil)
	_ device.Interrupter     = (*sink)(nil)
	_ device.StatsReporter   = (*sink)(nil)
)

// newSink opens the named device (empty => "default") and sets canonical params.
// Called only when the init() probe succeeded (bound != nil).
func newSink(dev string, log *slog.Logger) (*sink, error) {
	if bound == nil {
		return nil, fmt.Errorf("alsa: libasound not loaded")
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.With("backend", "alsa") // comp=sink already attributed by the registry

	if dev == "" {
		dev = "default"
	}
	var pcm uintptr
	if rc := bound.open(&pcm, dev, sndPCMStreamPlayback, 0); rc < 0 {
		return nil, fmt.Errorf("alsa: snd_pcm_open(%s) failed: %d", dev, rc)
	}

	// Small FIXED device latency — scheduling slack + something for the blocking write
	// to block on, NOT a jitter budget. NETWORK jitter is absorbed in our own INPUT
	// jitter buffer (the device buffer is downstream of the resampler and cannot help
	// with it); this buffer only covers OUR scheduling jitter (the playout goroutine
	// being late to feed the DAC). ~40 ms (2 frames) is enough on Pi-class hardware;
	// rough hosts can raise it via ENSEMBLE_ALSA_LATENCY_MS (20..500). A constant
	// inter-device difference is what the per-node outputDelayMs calibration is for
	// (D36). The playout regulates the play head against the master clock and folds THIS
	// value in as a constant offset, so its exact size does not affect loop stability.
	latencyMs := configuredLatencyMs()
	latencyUs := int32(latencyMs * 1000)
	if rc := bound.setParams(pcm, sndPCMFormatS16LE, sndPCMAccessRWInterlvd,
		stream.Channels, stream.SampleRate, alsaSoftResample, latencyUs); rc < 0 {
		bound.close(pcm)
		return nil, fmt.Errorf("alsa: snd_pcm_set_params failed: %d", rc)
	}
	log.Info("alsa device opened", "device", dev, "latencyMs", latencyMs)
	return &sink{f: bound, pcm: pcm, latencyMs: latencyMs, log: log}, nil
}

// configuredLatencyMs reads ENSEMBLE_ALSA_LATENCY_MS (clamped to 20..500),
// defaulting to defaultLatencyMs (~40ms / 2 frames).
func configuredLatencyMs() int {
	if v := os.Getenv("ENSEMBLE_ALSA_LATENCY_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= minLatencyMs && ms <= maxLatencyMs {
			return ms
		}
	}
	return defaultLatencyMs
}

// Write plays one frame and BLOCKS until the device can take the next one — this
// backpressure IS the playout rate pacer (device package contract). snd_pcm_writei
// is naturally blocking and runs UNLOCKED so a concurrent Interrupt()/Flush()
// (snd_pcm_drop) can abort a parked write (see the package LOCKING MODEL).
func (s *sink) Write(frame []byte) error {
	if len(frame) != stream.FrameBytes {
		return fmt.Errorf("alsa: frame %d bytes, want %d", len(frame), stream.FrameBytes)
	}

	// Brief critical section: snapshot the handle + closed flag. Everything below
	// runs unlocked except the recover path, which re-takes the mutex.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("alsa: closed")
	}
	pcm := s.pcm
	s.mu.Unlock()

	buf := uintptr(unsafe.Pointer(&frame[0]))
	n := s.f.writei(pcm, buf, uintptr(stream.FrameSamples)) // BLOCKS, unlocked
	if n < 0 {
		return s.recoverAndRetry(pcm, buf, n)
	}

	s.mu.Lock()
	s.framesWr++
	s.mu.Unlock()
	return nil
}

// recoverAndRetry handles a writei failure: underrun (-EPIPE), suspend (-ESTRPIPE)
// or an Interrupt/Flush-induced drop. Every recovery is an AUDIBLE glitch the
// pipeline counters cannot see — it is logged loudly (rate-limited to 1/s) and
// counted into Underruns so "choppy but clean counters" is diagnosable from the log
// alone. The retry writei again runs UNLOCKED.
func (s *sink) recoverAndRetry(pcm, buf uintptr, n int) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("alsa: closed")
	}
	s.xruns++
	s.writeErrs++
	if now := time.Now(); now.Sub(s.lastXrunLog) > time.Second {
		s.lastXrunLog = now
		s.log.Warn("alsa xrun (device underrun) — audible glitch; consider a larger ENSEMBLE_ALSA_LATENCY_MS",
			"err", n, "xruns", s.xruns)
	}
	s.mu.Unlock()

	if rc := s.f.recover(pcm, int32(n), 1); rc < 0 {
		return fmt.Errorf("alsa: writei %d, recover %d", n, rc)
	}
	n = s.f.writei(pcm, buf, uintptr(stream.FrameSamples)) // BLOCKS, unlocked
	if n < 0 {
		s.mu.Lock()
		s.writeErrs++
		s.mu.Unlock()
		return fmt.Errorf("alsa: writei after recover: %d", n)
	}
	s.mu.Lock()
	s.framesWr++
	s.mu.Unlock()
	return nil
}

// Delay returns the queued audio between Write and the speaker in ns
// (device.DelayReporter) — the exact phase reference the servo locks to (§3.5). The
// integer maths is deliberately frames*FrameNanos/FrameSamples (no float). ok=false
// when the device is closed or snd_pcm_delay errors. Brief, never across a blocking
// call.
func (s *sink) Delay() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, false
	}
	var frames int
	if rc := s.f.delay(s.pcm, &frames); rc < 0 {
		return 0, false
	}
	if frames < 0 {
		frames = 0
	}
	ns := int64(frames) * stream.FrameNanos / int64(stream.FrameSamples)
	s.lastDelayNs = ns
	return ns, true
}

// ConfiguredLatencyNs reports the device buffer the sink was opened with
// (device.LatencyReporter). The playout writes this far ahead so the device is
// pre-rolled to its full buffer (xrun cushion) and subtracts it from the deadline so
// nodes with different device latencies still play in phase (D63).
func (s *sink) ConfiguredLatencyNs() int64 {
	return int64(s.latencyMs) * 1_000_000
}

// Flush drops queued-but-unplayed audio and re-prepares the device (device.Flusher):
// a session ended — whatever the device retains must never replay at the next
// session's start. snd_pcm_drop runs UNLOCKED so it can also abort a writei parked in
// another goroutine.
func (s *sink) Flush() {
	pcm, ok := s.handle()
	if !ok {
		return
	}
	if rc := s.f.drop(pcm); rc < 0 {
		s.log.Debug("alsa drop failed", "rc", rc)
	}
	if rc := s.f.prepare(pcm); rc < 0 {
		s.log.Debug("alsa prepare after drop failed", "rc", rc)
	}
}

// Interrupt aborts an in-flight blocking Write (device.Interrupter): snd_pcm_drop
// unsticks a writei parked waiting for buffer space, so Close/Reset stay snappy and a
// wedged device cannot deadlock shutdown. Safe to call from ANOTHER goroutine — drop
// runs UNLOCKED on the snapshotted handle (see the package LOCKING MODEL). It does
// NOT re-prepare: the device is being torn down or re-anchored; the next Write's
// recover path or a Flush re-prepares as needed.
func (s *sink) Interrupt() {
	pcm, ok := s.handle()
	if !ok {
		return
	}
	if rc := s.f.drop(pcm); rc < 0 {
		s.log.Debug("alsa interrupt drop failed", "rc", rc)
	}
}

// Close stops the device and releases the handle (idempotent). The engine guarantees
// Interrupt-before-Close-before-the-writer-exits, so no writei is in flight here; the
// closed flag merely fences any later Write/Delay/Flush/Interrupt off the freed
// handle.
func (s *sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if rc := s.f.close(s.pcm); rc < 0 {
		return fmt.Errorf("alsa: snd_pcm_close: %d", rc)
	}
	return nil
}

// DeviceStats reports the adapter's telemetry (device.StatsReporter). The queue is a
// true measurement here (QueueValid), carrying the most recent Delay() reading.
func (s *sink) DeviceStats() device.DeviceStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return device.DeviceStats{
		Kind:                "alsa",
		QueueNs:             s.lastDelayNs,
		QueueValid:          true,
		ConfiguredLatencyNs: int64(s.latencyMs) * 1_000_000,
		FramesWritten:       s.framesWr,
		WriteErrors:         s.writeErrs,
		Underruns:           s.xruns,
	}
}

// handle snapshots the pcm handle, reporting ok=false if the sink is closed. The
// caller then runs the (possibly blocking) libasound call UNLOCKED on the snapshot.
func (s *sink) handle() (uintptr, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, false
	}
	return s.pcm, true
}
