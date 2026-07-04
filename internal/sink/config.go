package sink

import (
	"log/slog"
	"time"

	"ondaire/internal/contracts"
	"ondaire/internal/sink/device"
)

// RestartFunc is the watchdog's escape hatch (§8.6). When playout starves for
// the Watchdog interval (2 s) the sink calls it once before disarming; G's
// subscriber turns it into a wire RESTART to the source ("I got lost, re-prime
// me"). The sink itself never touches the network. nil is allowed (no-op).
type RestartFunc func()

// Config configures one Playout for one session-capable member. Constructed
// once per node; BufferMs and Gen refresh per session via Reset/SetBufferMs.
type Config struct {
	Backend  device.Sink     // output device; never nil
	Clock    contracts.Clock // master-time translation (F); never nil
	BufferMs int             // playout lead: audio for pts hits device at pts+BufferMs (§8.5)
	Restart  RestartFunc     // watchdog hook (§8.6); may be nil
	Log      *slog.Logger    // component logger; defaulted if nil

	// Per-node calibration; initial values come from node.json via K (D35/D36).
	// Volume is AUTHORITATIVE as given: 0.0 is a genuinely muted node, no
	// remapping. Tests must set 1.0 explicitly for unity.
	Volume        float64 // initial software gain 0.0–1.0 (D35)
	OutputDelayMs int     // initial output-delay calibration, ms (D36); clamped ±500
	Channel       string  // initial playout channel: "stereo" (default) | "L" | "R" (dual-mono)

	// Tunables; zero => default (overridable in tests).
	Capacity int           // jitter-buffer slot cap (default 256 frames ≈ 5.1 s)
	Watchdog time.Duration // starvation timeout (default 2 s, §8.6)
	now      func() int64  // local monotonic ns; default monotoNow (tests inject)
	servoCfg servoConfig   // LP-P gain / filter / clamps; default tuned values (tests override)
}

const (
	defaultCapacity = 256
	defaultWatchdog = 2 * time.Second
	maxDelayMs      = 500 // ±500 ms output-delay clamp (D36, §1)
)
