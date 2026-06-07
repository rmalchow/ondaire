package audio

import (
	"context"
	"io"
	"sync"
	"time"

	"ensemble/internal/stream"
)

// readaheadFrames bounds the producer channel (~1 s at 20 ms/frame).
const readaheadFrames = 50

// frameDeadline is one frame period: if no frame is buffered within it,
// ReadFrame returns silence rather than stalling (§6.1).
var frameDeadline = time.Duration(stream.FrameDuration) * time.Millisecond

// liveReader adapts a pull framer over an arbitrary byte stream into the
// live-paced Source semantics (never EOF; underflow→silence). Used by both
// httpSource and inputSource.
type liveReader struct {
	frames chan [stream.FrameBytes]byte
	closed chan struct{}
	done   chan struct{} // producer exited

	mu      sync.Mutex
	loadErr error

	once sync.Once
	stop context.CancelFunc

	// cleanup runs once on Close (after the producer exits): closes byte source.
	cleanup func()
}

// newLiveReader starts a producer goroutine that frames src into a bounded
// channel. stop is the context-cancel that tears down the byte stream; cleanup
// (may be nil) runs once after the producer exits.
func newLiveReader(fr *framer, stop context.CancelFunc, cleanup func()) *liveReader {
	lr := &liveReader{
		frames:  make(chan [stream.FrameBytes]byte, readaheadFrames),
		closed:  make(chan struct{}),
		done:    make(chan struct{}),
		stop:    stop,
		cleanup: cleanup,
	}
	go lr.produce(fr)
	return lr
}

func (lr *liveReader) produce(fr *framer) {
	defer close(lr.done)
	defer close(lr.frames)
	for {
		var f [stream.FrameBytes]byte
		err := fr.frame(f[:])
		if err == io.EOF {
			return // finite body drained; consumer keeps emitting silence
		}
		if err != nil {
			lr.setErr(err)
			return
		}
		select {
		case lr.frames <- f:
		case <-lr.closed:
			return
		}
	}
}

func (lr *liveReader) setErr(err error) {
	lr.mu.Lock()
	lr.loadErr = err
	lr.mu.Unlock()
}

func (lr *liveReader) fatalErr() error {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	return lr.loadErr
}

// ReadFrame is the live pacer: it returns a buffered frame, or a silence frame
// on underflow, and never returns io.EOF until Close.
func (lr *liveReader) ReadFrame(dst []byte) error {
	select {
	case <-lr.closed:
		return io.EOF
	default:
	}

	select {
	case f, ok := <-lr.frames:
		if !ok { // producer gone
			if err := lr.fatalErr(); err != nil {
				return err
			}
			fillSilence(dst)
			return nil
		}
		copy(dst, f[:])
		return nil
	case <-time.After(frameDeadline):
		fillSilence(dst)
		return nil
	case <-lr.closed:
		return io.EOF
	}
}

func (lr *liveReader) Live() bool { return true }

// Close cancels the byte stream, unblocks ReadFrame, waits for the producer,
// and runs cleanup. Idempotent.
func (lr *liveReader) Close() error {
	lr.once.Do(func() {
		close(lr.closed)
		if lr.stop != nil {
			lr.stop()
		}
		<-lr.done
		if lr.cleanup != nil {
			lr.cleanup()
		}
	})
	return nil
}

func fillSilence(dst []byte) {
	d := dst[:stream.FrameBytes]
	for i := range d {
		d[i] = 0
	}
}
