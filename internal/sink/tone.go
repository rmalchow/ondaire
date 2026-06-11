package sink

import (
	"errors"
	"math"
	"time"

	"ensemble/internal/stream"
)

// ErrBusy is returned by TestTone when a session is active or a tone is
// already playing.
var ErrBusy = errors.New("sink: busy (session active or tone already playing)")

// TestTone plays a short 440 Hz tone directly through the output backend —
// a bring-up aid (UI "test tone" button) to verify a node's audio path
// without starting a session. Refused while a session is armed. The tone
// respects the node's live volume (gain stage). Asynchronous: returns once
// started; the writer goroutine stops early on Close or session arm.
func (p *Playout) TestTone(d time.Duration) error {
	if d <= 0 || d > 5*time.Second {
		d = time.Second
	}
	p.mu.Lock()
	if p.closed || p.armed || p.toneBusy {
		p.mu.Unlock()
		return ErrBusy
	}
	p.toneBusy = true
	out := p.out
	p.mu.Unlock()

	// A test tone is an operator poking a node: if the output has given up and is
	// resting after repeated failures, force it to retry the chain now so the tone
	// has a chance of being heard (the retry may still land on a dead output).
	if rv, ok := out.(interface{ Revive() }); ok {
		rv.Revive()
	}

	frames := int(d / (stream.FrameDuration * time.Millisecond))
	p.log.Info("test tone", "ms", d.Milliseconds())

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() {
			p.mu.Lock()
			p.toneBusy = false
			p.mu.Unlock()
		}()
		frame := make([]byte, stream.FrameBytes)
		n := 0
		for f := 0; f < frames; f++ {
			p.mu.Lock()
			stop := p.closed || p.armed
			p.mu.Unlock()
			if stop {
				return
			}
			for i := 0; i < stream.FrameSamples; i++ {
				v := int16(0.25 * 32767 * math.Sin(2*math.Pi*440*float64(n)/float64(stream.SampleRate)))
				frame[i*4+0] = byte(v)
				frame[i*4+1] = byte(v >> 8)
				frame[i*4+2] = byte(v)
				frame[i*4+3] = byte(v >> 8)
				n++
			}
			p.gain.apply(frame)
			if err := out.Write(frame); err != nil {
				p.log.Warn("test tone write failed", "err", err)
				return
			}
		}
	}()
	return nil
}
