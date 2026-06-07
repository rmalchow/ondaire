// Command soundcheck is a temporary diagnostic: it plays a short tone through
// the named sink backend so we can hear which output path actually reaches the
// speakers. Usage: soundcheck <alsa|exec> [device]
package main

import (
	"fmt"
	"math"
	"os"
	"time"

	"ensemble/internal/sink"
	"ensemble/internal/stream"
)

func main() {
	name := "alsa"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	backend, kind, err := sink.Open(name, nil)
	if err != nil {
		fmt.Printf("RESULT %s: open failed: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("opened backend %s (%v)\n", name, kind)

	// 0.7 s of 440 Hz stereo 48k s16le in 20 ms frames.
	frames := 35
	frame := make([]byte, stream.FrameBytes)
	n := 0
	for f := 0; f < frames; f++ {
		for i := 0; i < stream.FrameSamples; i++ {
			v := int16(0.25 * 32767 * math.Sin(2*math.Pi*440*float64(n)/48000))
			frame[i*4+0] = byte(v)
			frame[i*4+1] = byte(v >> 8)
			frame[i*4+2] = byte(v)
			frame[i*4+3] = byte(v >> 8)
			n++
		}
		if err := backend.Write(frame); err != nil {
			fmt.Printf("RESULT %s: write %d failed: %v\n", name, f, err)
			_ = backend.Close()
			os.Exit(1)
		}
	}
	time.Sleep(300 * time.Millisecond)
	_ = backend.Close()
	fmt.Printf("RESULT %s: wrote %d frames OK\n", name, frames)
}
