package audio

import (
	"context"
	"net/url"
	"strconv"

	"ensemble/internal/stream"
)

// SchemeCalib is an INTERNAL media scheme for the by-ear alignment helper — it is
// not advertised in Schemes() (not a user media source). A calib: URI plays a
// synchronized test signal through the normal group/source path so a user can
// null the inter-speaker flam by adjusting each node's OutputDelayMs:
//
//	calib:click?hz=2&level=0.5   — sharp broadband click train (default; the
//	                               transient flams when misaligned, combs sub-5 ms)
//	calib:noise?level=0.4        — correlated pink-ish noise for the final ~ms,
//	                               nulled by minimizing comb-filter coloration
const SchemeCalib = "calib"

func init() { registry[SchemeCalib] = openCalib }

// openCalib parses a calib: URI into the matching synthetic source. The signal is
// generated on the master and fanned out with deterministic PTS, so every node
// emits it at the same master-clock instant (alignment is then purely the per-node
// output-delay). ctx/mediaDir are unused — nothing is read from disk.
func openCalib(_ context.Context, uri, _ string) (Source, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, ErrBadMedia
	}
	mode := u.Opaque // the bit between "calib:" and "?"
	if mode == "" {
		mode = "click"
	}
	q := u.Query()
	level := clampF(parseF(q.Get("level"), 0.5), 0.02, 1.0)
	switch mode {
	case "click":
		return newClickTrain(clampI(parseI(q.Get("hz"), 2), 1, 20), level), nil
	case "noise":
		return newCalibNoise(level), nil
	default:
		return nil, ErrUnsupportedFormat
	}
}

// clickTrain emits a crisp broadband tick every period and silence in between.
// The tick is a fast-onset, fast-decay noise burst (~2 ms): the sharp onset gives
// a clean flam for medium offsets, the broadband content combs the timbre for
// sub-5 ms ones. Owned by one goroutine (the release ticker); not concurrent-safe.
type clickTrain struct {
	period int     // samples between click onsets
	click  int     // click length in samples
	level  float64 // 0..1 peak
	n      int64   // absolute sample index
	lcg    uint32  // tiny PRNG for the broadband burst
}

func newClickTrain(hz int, level float64) *clickTrain {
	period := stream.SampleRate / hz
	click := stream.SampleRate * 2 / 1000 // ~2 ms
	if click > period {
		click = period
	}
	return &clickTrain{period: period, click: click, level: level, lcg: 0x12345678}
}

func (c *clickTrain) Live() bool   { return true }
func (c *clickTrain) Close() error { return nil }

func (c *clickTrain) ReadFrame(dst []byte) error {
	for i := 0; i < stream.FrameSamples; i++ {
		var v int16
		if phase := int(c.n % int64(c.period)); phase < c.click {
			env := 1.0 - float64(phase)/float64(c.click) // sharp onset, linear decay
			c.lcg = c.lcg*1664525 + 1013904223
			noise := float64(int32(c.lcg)) / 2147483648.0 // -1..1
			v = pcm16(c.level * env * noise)
		}
		putLR(dst[i*4:], v) // identical L/R so it sums coherently in a room
		c.n++
	}
	return nil
}

// calibNoise emits continuous correlated (identical L/R) pink-ish noise — for the
// final null: when two speakers are aligned the comb filter pushes past the audio
// band and the sound is fullest/brightest; misaligned it sounds hollow/phasey.
type calibNoise struct {
	level float64
	lcg   uint32
	lp    float64 // one-pole low-pass state → softer, pink-ish than raw white
}

func newCalibNoise(level float64) *calibNoise { return &calibNoise{level: level, lcg: 0x2468ace0} }

func (c *calibNoise) Live() bool   { return true }
func (c *calibNoise) Close() error { return nil }

func (c *calibNoise) ReadFrame(dst []byte) error {
	for i := 0; i < stream.FrameSamples; i++ {
		c.lcg = c.lcg*1664525 + 1013904223
		white := float64(int32(c.lcg)) / 2147483648.0
		c.lp += 0.05 * (white - c.lp)             // one-pole LP
		putLR(dst[i*4:], pcm16(c.level*c.lp*3.0)) // *3 compensates the LP attenuation
	}
	return nil
}

// putLR writes one stereo s16le sample-pair (same value both channels) into b[:4].
func putLR(b []byte, v int16) {
	b[0], b[1] = byte(v), byte(v>>8)
	b[2], b[3] = byte(v), byte(v>>8)
}

func pcm16(x float64) int16 {
	if x > 1 {
		x = 1
	} else if x < -1 {
		x = -1
	}
	return int16(x * 32767)
}

func parseF(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return def
}

func parseI(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
