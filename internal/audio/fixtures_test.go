package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeWAVs16 returns a minimal RIFF/WAVE PCM s16le file with the given samples
// (interleaved if channels==2).
func writeWAVs16(rate, channels int, samples []int16) []byte {
	data := new(bytes.Buffer)
	for _, s := range samples {
		binary.Write(data, binary.LittleEndian, s)
	}
	return wavContainer(rate, channels, 16, wavPCM, data.Bytes())
}

// writeWAVu8 returns an 8-bit unsigned PCM WAV.
func writeWAVu8(rate, channels int, samples []uint8) []byte {
	return wavContainer(rate, channels, 8, wavPCM, samples)
}

// writeWAVs24 returns a 24-bit signed PCM WAV from int32 samples (low 24 bits).
func writeWAVs24(rate, channels int, samples []int32) []byte {
	data := new(bytes.Buffer)
	for _, s := range samples {
		data.WriteByte(byte(s))
		data.WriteByte(byte(s >> 8))
		data.WriteByte(byte(s >> 16))
	}
	return wavContainer(rate, channels, 24, wavPCM, data.Bytes())
}

// writeWAVfloat32 returns a 32-bit IEEE-float WAV.
func writeWAVfloat32(rate, channels int, samples []float32) []byte {
	data := new(bytes.Buffer)
	for _, s := range samples {
		binary.Write(data, binary.LittleEndian, math.Float32bits(s))
	}
	return wavContainer(rate, channels, 32, wavIEEEFloat, data.Bytes())
}

func writeU32(b *bytes.Buffer, v uint32) { binary.Write(b, binary.LittleEndian, v) }
func writeU16(b *bytes.Buffer, v uint16) { binary.Write(b, binary.LittleEndian, v) }

func wavContainer(rate, channels, bits, format int, data []byte) []byte {
	blockAlign := channels * bits / 8
	byteRate := rate * blockAlign

	b := new(bytes.Buffer)
	b.WriteString("RIFF")
	binary.Write(b, binary.LittleEndian, uint32(36+len(data)))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	binary.Write(b, binary.LittleEndian, uint32(16))
	binary.Write(b, binary.LittleEndian, uint16(format))
	binary.Write(b, binary.LittleEndian, uint16(channels))
	binary.Write(b, binary.LittleEndian, uint32(rate))
	binary.Write(b, binary.LittleEndian, uint32(byteRate))
	binary.Write(b, binary.LittleEndian, uint16(blockAlign))
	binary.Write(b, binary.LittleEndian, uint16(bits))
	b.WriteString("data")
	binary.Write(b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

// genTone produces deterministic interleaved int16 samples of a sine tone.
func genTone(rate, channels int, freq float64, nFrames int) []int16 {
	out := make([]int16, 0, nFrames*channels)
	for i := 0; i < nFrames; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/float64(rate)) * 16000)
		for c := 0; c < channels; c++ {
			out = append(out, v)
		}
	}
	return out
}

// fakeCaptureExe builds a tiny helper binary that emits raw s16le on stdout.
// Args (env): FAKE_FRAMES = number of stereo frames to emit; FAKE_STALL=1 makes
// it emit a burst, then block forever (to test stall→silence and Close→kill).
func fakeCaptureExe(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available to build fake capture exe")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "fakecap.go")
	if err := os.WriteFile(src, []byte(fakeCaptureSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "fakecap")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake capture: %v\n%s", err, out)
	}
	return bin
}

const fakeCaptureSrc = `package main

import (
	"encoding/binary"
	"os"
	"strconv"
	"time"
)

func main() {
	frames := 480 // default ~ 10 frames worth
	if v := os.Getenv("FAKE_FRAMES"); v != "" {
		frames, _ = strconv.Atoi(v)
	}
	stall := os.Getenv("FAKE_STALL") == "1"
	buf := make([]byte, 0, frames*4)
	for i := 0; i < frames; i++ {
		var s int16 = int16((i % 100) * 100)
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(s))
		buf = append(buf, b[0], b[1], b[0], b[1]) // stereo
	}
	os.Stdout.Write(buf)
	if stall {
		for {
			time.Sleep(time.Hour)
		}
	}
}
`

// isSilent reports whether all bytes are zero.
func isSilent(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// maybeFixture returns the path to an optional testdata file and whether to
// skip (file absent).
func maybeFixture(t *testing.T, name string) (string, bool) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err != nil {
		return "", true
	}
	return p, false
}
