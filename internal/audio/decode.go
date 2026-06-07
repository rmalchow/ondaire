package audio

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"ensemble/internal/stream"
)

// sampleReader is a native-rate, native-channel PCM sample producer — the
// single seam over the three decode libs (wav/mp3/flac).
type sampleReader interface {
	// info reports native sample rate (Hz) and channel count (1 or 2).
	info() (sampleRate, channels int)
	// read appends interleaved int16 samples to dst and returns the grown
	// slice. May return data with io.EOF, or io.EOF alone when drained. Any
	// other error is a decode failure.
	read(dst []int16) ([]int16, error)
	io.Closer
}

// newDecoder picks a sampleReader by media format and wraps r. format is
// "wav"/"mp3"/"flac"; empty/unknown triggers a 12-byte sniff before giving up
// with ErrUnsupportedFormat.
func newDecoder(r io.Reader, format string) (sampleReader, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "wav", "wave":
		return newWAVSource(r)
	case "mp3", "mpeg":
		return newMP3Source(r)
	case "flac":
		return newFLACSource(r)
	}

	// Unknown format: sniff the first bytes. Wrap r so the sniffed bytes are
	// not lost to the chosen decoder.
	br := bufio.NewReaderSize(r, 4096)
	head, _ := br.Peek(12)
	switch {
	case len(head) >= 4 && string(head[0:4]) == "RIFF":
		return newWAVSource(br)
	case len(head) >= 4 && string(head[0:4]) == "fLaC":
		return newFLACSource(br)
	case len(head) >= 3 && string(head[0:3]) == "ID3":
		return newMP3Source(br)
	case len(head) >= 2 && head[0] == 0xFF && (head[1]&0xE0) == 0xE0:
		return newMP3Source(br) // raw MPEG sync
	}
	return nil, fmt.Errorf("%w: could not identify media", ErrUnsupportedFormat)
}

// framer pulls native int16 samples from src, mono-dups, resamples to 48 kHz,
// and slices canonical 20 ms frames into caller-owned dst.
type framer struct {
	src      sampleReader
	rs       *resampler
	channels int
	canon    []int16 // accumulated canonical (48k, stereo) samples
	scratch  []int16 // native-read scratch
	idx      uint64
	eof      bool // src drained
	done     bool // final padded frame already emitted
}

func newFramer(src sampleReader) *framer {
	rate, ch := src.info()
	f := &framer{src: src, channels: ch}
	if rate != stream.SampleRate {
		f.rs = newResampler(rate)
	}
	return f
}

// fill pulls one batch from src, converts to canonical stereo 48k, and appends
// to f.canon. Sets f.eof when src is drained.
func (f *framer) fill() error {
	f.scratch = f.scratch[:0]
	var err error
	f.scratch, err = f.src.read(f.scratch)
	atEOF := err == io.EOF
	if err != nil && !atEOF {
		return err
	}

	stereo := f.scratch
	if f.channels == 1 {
		// mono → dup to stereo
		dup := make([]int16, 0, len(f.scratch)*2)
		for _, s := range f.scratch {
			dup = append(dup, s, s)
		}
		stereo = dup
	}

	if f.rs != nil {
		f.canon = f.rs.process(stereo, atEOF, f.canon)
	} else {
		f.canon = append(f.canon, stereo...)
	}

	if atEOF {
		f.eof = true
	}
	return nil
}

// frame fills dst[:stream.FrameBytes] with the next canonical frame, returning
// nil per frame and io.EOF (no write) once drained (D9).
func (f *framer) frame(dst []byte) error {
	const frameSamples = stream.FrameSamples * stream.Channels // 1920 int16

	for len(f.canon) < frameSamples && !f.eof {
		if err := f.fill(); err != nil {
			return err
		}
	}

	if len(f.canon) == 0 {
		// Drained.
		return io.EOF
	}

	n := frameSamples
	if n > len(f.canon) {
		n = len(f.canon)
	}
	// Write n samples, then zero-pad the remainder of the frame.
	for i := 0; i < n; i++ {
		v := uint16(f.canon[i])
		dst[i*2] = byte(v)
		dst[i*2+1] = byte(v >> 8)
	}
	for i := n; i < frameSamples; i++ {
		dst[i*2] = 0
		dst[i*2+1] = 0
	}
	// Drop consumed samples.
	f.canon = f.canon[n:]
	if len(f.canon) == 0 {
		f.canon = f.canon[:0]
	}
	f.idx++
	return nil
}
