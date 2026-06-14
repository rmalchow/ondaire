package device

// DeviceStats is the telemetry an adapter exposes via StatsReporter — the single
// surface the engine merges into the cross-piece SinkStats and into the richer,
// local-only /api/status debug view.
//
// APPEND-ONLY: add fields at the bottom. Consumers read by name, so growth never
// breaks anything; in particular it never silently widens the STATUS wire datagram,
// which carries only a deliberately-chosen subset (a wire change is an explicit
// edit to stream/control.go plus a version bump).
type DeviceStats struct {
	Kind                string // live device kind: "alsa" | "exec" | "file" | "null"
	QueueNs             int64  // queued audio between Write and the speaker (== Delay)
	QueueValid          bool   // QueueNs is a true measurement (alsa/file), not none/estimate (exec/null)
	ConfiguredLatencyNs int64  // prime/target buffer depth; 0 if not applicable
	FramesWritten       uint64 // frames accepted by the device
	WriteErrors         uint64 // write failures observed
	Underruns           uint64 // alsa xruns; exec player respawns; file/null: 0

	// Wrapper-only (the resilient failover backend); zero for leaf adapters.
	Rotations uint64 // failover candidate switches
	Resting   bool   // in failover backoff (discarding audio)
	BackoffMs int64  // current backoff window when resting

	// Append future metrics below (DeviceName, SampleRate, JitterNs, …). They reach
	// logs and /api/status automatically; they reach the WIRE only via a deliberate
	// stream/control.go change.
}
