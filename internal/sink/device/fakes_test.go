package device

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"ensemble/internal/stream"
)

// This file wires a fixed set of FAKE backend kinds into the package-global
// registry exactly once (registry.Register panics on a duplicate name, and the
// real alsa/exec/file/null adapters are NOT imported into the device test binary,
// so these names are free). Tests share these kinds; per-test behaviour is
// isolated by the *fakeCtl an opened sink is bound to, looked up by the control id
// carried in the candidate Arg — so two tests never fight over one mutable knob.
//
// Registered kinds:
//
//	"null"            real-output stand-in #0: a paced-free discard sink (the
//	                  resilient wrapper and OpenResilient open this internally).
//	"file"            discard sink reused for the file:/ bypass path in open_test.
//	"tk"              the controllable failover candidate: factory(arg) binds the
//	                  returned *fakeSink to fakeCtls[arg].
//	"auto_alsa"       availability-gated fake used by open_test's auto chain as the
//	                  "preferred real kind"; gate via setAvailable.
//	"auto_exec"       second auto-chain fake (lower preference than auto_alsa).
//	"named_explicit"  a plain registered kind for the explicit-name open path.
//	"rk_real"         a non-null/file kind for HasPlayback/BackendNames assertions,
//	                  availability-gated.

const fakeFrameSize = stream.FrameBytes

func goodFrame() []byte { return make([]byte, fakeFrameSize) }

// fakeCtl is the controllable behaviour of one opened fake sink. Tests create one,
// stash it under a string id, and put that id in the candidate Arg.
type fakeCtl struct {
	openErr     error        // if set, the factory returns this instead of a sink
	failWrites  atomic.Bool  // when true, Write returns an error (player "down")
	failAfter   atomic.Int64 // >0: succeed this many writes, then fail forever
	opens       atomic.Int64 // times the factory opened a sink for this ctl
	writes      atomic.Int64 // successful writes across all sinks for this ctl
	closes      atomic.Int64 // Close calls
	implsDelay  bool         // the opened sink also implements DelayReporter
	delayNs     int64        // value Delay() returns when implsDelay
	delayOK     bool         // ok flag Delay() returns
	kindReport  string       // Kind reported by DeviceStats (defaults to "tk")
	framesStats atomic.Uint64
}

var (
	fakeCtlMu sync.Mutex
	fakeCtls  = map[string]*fakeCtl{}
)

func putCtl(id string, c *fakeCtl) {
	fakeCtlMu.Lock()
	fakeCtls[id] = c
	fakeCtlMu.Unlock()
}

func getCtl(id string) *fakeCtl {
	fakeCtlMu.Lock()
	defer fakeCtlMu.Unlock()
	return fakeCtls[id]
}

// fakeSink is the plain controllable sink (no extra capabilities). It is the base
// returned by the "tk" factory when the ctl does not request DelayReporter/Stats.
type fakeSink struct {
	ctl *fakeCtl
}

func (s *fakeSink) Write(frame []byte) error {
	if len(frame) != fakeFrameSize {
		return fmt.Errorf("fake: frame %d bytes, want %d", len(frame), fakeFrameSize)
	}
	if s.ctl.failWrites.Load() {
		return fmt.Errorf("fake: down")
	}
	if n := s.ctl.failAfter.Load(); n > 0 {
		if s.ctl.writes.Load() >= n {
			return fmt.Errorf("fake: down after %d", n)
		}
	}
	s.ctl.writes.Add(1)
	s.ctl.framesStats.Add(1)
	return nil
}

func (s *fakeSink) Close() error { s.ctl.closes.Add(1); return nil }

// DeviceStats — fakeSink is always a StatsReporter so the live leaf reports a Kind
// (the wrapper overlays its own fields on top). Kind defaults to "tk".
func (s *fakeSink) DeviceStats() DeviceStats {
	k := s.ctl.kindReport
	if k == "" {
		k = "tk"
	}
	return DeviceStats{Kind: k, FramesWritten: s.ctl.framesStats.Load(), QueueValid: true}
}

// fakeDelaySink additionally implements DelayReporter — used to prove device.Query
// reaches a live candidate's Delay() through the wrapper's As escape hatch.
type fakeDelaySink struct {
	fakeSink
}

func (s *fakeDelaySink) Delay() (int64, bool) { return s.ctl.delayNs, s.ctl.delayOK }

// discardSink is a trivial always-succeeds sink for the "null"/"file" kinds. It
// validates frame size (like the real null) so a mis-sized frame is still caught.
type discardSink struct {
	kind   string
	arg    string // factory arg the sink was opened with (e.g. the alsa device id)
	closed atomic.Bool
}

func (d *discardSink) Write(frame []byte) error {
	if len(frame) != fakeFrameSize {
		return fmt.Errorf("%s: frame %d bytes, want %d", d.kind, len(frame), fakeFrameSize)
	}
	return nil
}
func (d *discardSink) Close() error { d.closed.Store(true); return nil }
func (d *discardSink) DeviceStats() DeviceStats {
	return DeviceStats{Kind: d.kind, QueueValid: false}
}

// availFlags gates availability-sensitive kinds per test.
var availFlags = struct {
	mu sync.Mutex
	m  map[string]bool
}{m: map[string]bool{}}

func setAvailable(kind string, ok bool) {
	availFlags.mu.Lock()
	availFlags.m[kind] = ok
	availFlags.mu.Unlock()
}
func isAvailable(kind string) bool {
	availFlags.mu.Lock()
	defer availFlags.mu.Unlock()
	return availFlags.m[kind]
}

func init() {
	// "null"/"file": always-available discard sinks (no available gate).
	Register("null", func(_ string, _ *slog.Logger) (Sink, error) {
		return &discardSink{kind: "null"}, nil
	}, nil)
	Register("file", func(arg string, _ *slog.Logger) (Sink, error) {
		if arg == "" {
			return nil, fmt.Errorf("file: empty path")
		}
		return &discardSink{kind: "file"}, nil
	}, nil)

	// "tk": the controllable failover candidate. Arg is a control id. It is opened
	// only via openFactory (the failover chain), never via the available()-gated
	// auto path, so we report it UNAVAILABLE by default — that keeps it (and every
	// other generic test kind) out of HasPlayback's "any real output" OR, so the
	// HasPlayback assertions stay deterministic.
	Register("tk", func(arg string, _ *slog.Logger) (Sink, error) {
		c := getCtl(arg)
		if c == nil {
			return nil, fmt.Errorf("tk: no control %q", arg)
		}
		c.opens.Add(1)
		if c.openErr != nil {
			return nil, c.openErr
		}
		if c.implsDelay {
			return &fakeDelaySink{fakeSink{ctl: c}}, nil
		}
		return &fakeSink{ctl: c}, nil
	}, func() bool { return isAvailable("tk") })

	// Availability-gated stand-ins for the REAL "alsa"/"exec" kinds the open.go auto
	// chain hard-codes. The real adapters are not imported into the device test
	// binary, so these names are free; open.go's available("alsa")/openFactory(
	// "alsa",..) (and exec) resolve to these. Gate them with setAvailable.
	//
	// "alsa" honours the explicit-device arg by recording it on the sink so the
	// OpenDevice("alsa", dev, ..) routing can be asserted.
	Register("alsa", func(arg string, _ *slog.Logger) (Sink, error) {
		if !isAvailable("alsa") {
			return nil, fmt.Errorf("alsa: unavailable")
		}
		return &discardSink{kind: "alsa", arg: arg}, nil
	}, func() bool { return isAvailable("alsa") })

	Register("exec", func(_ string, _ *slog.Logger) (Sink, error) {
		if !isAvailable("exec") {
			return nil, fmt.Errorf("exec: unavailable")
		}
		return &discardSink{kind: "exec"}, nil
	}, func() bool { return isAvailable("exec") })

	// Gated unavailable by default so it stays out of HasPlayback; it is opened by
	// the explicit-name Open path (openFactory), which ignores availability.
	Register("named_explicit", func(_ string, _ *slog.Logger) (Sink, error) {
		return &discardSink{kind: "named_explicit"}, nil
	}, func() bool { return isAvailable("named_explicit") })

	Register("rk_real", func(_ string, _ *slog.Logger) (Sink, error) {
		return &discardSink{kind: "rk_real"}, nil
	}, func() bool { return isAvailable("rk_real") })
}

// tkCand builds a "tk" failover candidate bound to ctl id.
func tkCand(id string) Candidate {
	return Candidate{Kind: "tk", Arg: id, Label: "tk(" + id + ")"}
}

// uniqueName yields a kind name unused so far in this test binary, so tests that
// must call the panic-on-dup Register stay safe under `go test -count=N` (the
// global registry has no unregister).
var uniqueCtr atomic.Int64

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, uniqueCtr.Add(1))
}

// driveWrites pushes n good frames through s, ignoring the (swallowed) errors.
func driveWrites(s Sink, n int) {
	for i := 0; i < n; i++ {
		_ = s.Write(goodFrame())
	}
}

// stepClock is a manually-advanced clock for deterministic backoff timing.
type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func newStepClock() *stepClock { return &stepClock{t: time.Unix(1_700_000_000, 0)} }
func (c *stepClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}
