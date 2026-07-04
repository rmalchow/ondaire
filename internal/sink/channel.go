package sink

import (
	"sync/atomic"

	"ondaire/internal/stream"
)

// Channel modes for dual-mono playout.
const (
	chStereo int32 = 0 // both channels as received (default)
	chLeft   int32 = 1 // play the LEFT channel on both speakers
	chRight  int32 = 2 // play the RIGHT channel on both speakers
)

// channelStage optionally collapses the stereo output frame to DUAL-MONO: the
// chosen channel (L or R) is copied over BOTH output channels, so a node assigned
// a single channel plays that content on both its speakers. Stereo is a no-op
// (bit-identical passthrough). The mode is stored atomically so SetChannel is
// lock-free on the hot path. Applied AFTER gain, as the last per-sample transform
// before the device — the resampler/gain/device all stay stereo and unaware.
type channelStage struct {
	mode atomic.Int32
}

// parseChannel maps a config string to a mode (unknown/"" → stereo).
func parseChannel(s string) int32 {
	switch s {
	case "L":
		return chLeft
	case "R":
		return chRight
	default:
		return chStereo
	}
}

func newChannelStage(initial string) *channelStage {
	c := &channelStage{}
	c.mode.Store(parseChannel(initial))
	return c
}

// set stores a new mode atomically; lock-free, takes effect on the next frame.
func (c *channelStage) set(s string) { c.mode.Store(parseChannel(s)) }

// current returns the mode (for unchanged-guards).
func (c *channelStage) current() int32 { return c.mode.Load() }

// apply collapses frame in place to dual-mono per the current mode. Stereo →
// no-op. L → copy each sample-time's left sample over its right; R → the reverse.
// frame is interleaved s16le, FrameSamples·Channels samples. Scheduler goroutine only.
func (c *channelStage) apply(frame []byte) {
	mode := c.mode.Load()
	if mode == chStereo {
		return
	}
	const (
		n      = stream.FrameSamples
		bps    = stream.BytesPerSmpl
		stride = stream.Channels * bps // bytes per sample-time (4)
	)
	src, dst := 0, bps // L mode: src=left(0), dst=right(bps)
	if mode == chRight {
		src, dst = bps, 0
	}
	for i := 0; i < n; i++ {
		base := i * stride
		copy(frame[base+dst:base+dst+bps], frame[base+src:base+src+bps])
	}
}
