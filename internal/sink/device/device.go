// Package device defines the output-device port for the playout engine and hosts
// the device adapters (alsa/exec/file/null) as subpackages. The engine drives one
// Sink and makes no syscall itself; each adapter encapsulates exactly one real
// output and all of its host-specific baggage.
//
// CONTRACT — the one rule that makes the whole pipeline work: Sink.Write MUST block
// until the device can accept the next frame. That backpressure IS the playout RATE
// pacer; the engine has no other clock. A device without natural backpressure (file,
// null) synthesises it from an internal real-time clock. A Write that returns early
// spins the engine and is a bug.
//
// Everything beyond Write/Close is an OPTIONAL capability, discovered with Query and
// forwarded by wrappers (the resilient failover backend) via the As escape hatch. An
// adapter implements only what its hardware can honour; the engine degrades
// gracefully — e.g. no DelayReporter means no phase probe, so the servo holds the
// resample ratio near 1 and leans on the prime alignment plus the D36 calibration
// constant instead of a continuous phase lock.
package device

// Sink is one output device. Frames are canonical PCM, exactly stream.FrameBytes
// each (48 kHz stereo s16le, 20 ms).
type Sink interface {
	// Write plays one frame and BLOCKS until the device can take the next one (the
	// rate pacer — see the package contract). It returns an error only on a wrong
	// frame size or a permanently-gone device, never to signal mere backpressure.
	Write(frame []byte) error
	// Close stops the device and releases its resources. Idempotent.
	Close() error
}

// DelayReporter exposes the queued audio between Write and the speaker — the PHASE
// reference the servo locks to the master clock (content at the speaker now has
// master time fedPTS−Delay). ok=false means the latency is opaque (e.g. behind an
// external player): the engine then holds the ratio near 1, anchors phase once at
// prime, and trusts the D36 calibration constant rather than a continuous lock.
type DelayReporter interface {
	Delay() (ns int64, ok bool)
}

// LatencyReporter reports the device's configured buffer depth — the queue the
// engine fills to at startup so its first real frame lands in phase. 0 ⇒ unknown
// (the engine uses a default lead).
type LatencyReporter interface {
	ConfiguredLatencyNs() int64
}

// Flusher drops queued-but-unplayed audio on session end / re-anchor, so stale
// audio never replays at the next session's start.
type Flusher interface {
	Flush()
}

// Interrupter aborts an in-flight blocking Write so Close/Reset stay snappy and a
// wedged device cannot deadlock shutdown. An adapter that supports it must run its
// blocking write WITHOUT holding a lock the Interrupt path also needs.
type Interrupter interface {
	Interrupt()
}

// StatsReporter is the single telemetry surface from an adapter (see DeviceStats).
// The engine merges it into the cross-piece SinkStats; the resilient wrapper
// overlays the fields only it knows (live kind, failover health).
type StatsReporter interface {
	DeviceStats() DeviceStats
}

// DeviceSelector re-orders a device's internal preference — the UI device override
// (D37). Returns false when the device has nothing to select (e.g. exec/file/null).
type DeviceSelector interface {
	SetPreferred(device string) bool
}

// Reviver forces a device that has given up (failover backoff) to retry immediately
// — the UI test-tone poke.
type Reviver interface {
	Revive()
}

// ActiveReporter reports the live device kind whenever it changes, so the cluster
// record / UI can show what is actually playing. Implemented by the resilient
// wrapper; set once at wiring time.
type ActiveReporter interface {
	OnActive(fn func(kind string))
}

// Query returns s's T capability if present. A leaf adapter satisfies it by plain
// type assertion; a wrapper (resilient) satisfies it for its live delegate via the
// optional As(any) bool escape hatch — so a NEW capability flows through the wrapper
// with no wrapper edit (the pattern of errors.As / http.ResponseController).
//
//	if dr, ok := device.Query[device.DelayReporter](sink); ok { ... }
func Query[T any](s Sink) (T, bool) {
	if t, ok := s.(T); ok {
		return t, true
	}
	if a, ok := s.(interface{ As(target any) bool }); ok {
		var t T
		if a.As(&t) {
			return t, true
		}
	}
	var zero T
	return zero, false
}
