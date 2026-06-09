// Package stream defines the on-wire frame codec (wire.go), the STREAM_PORT
// UDP mux (mux.go), and — in later pieces — the source-subscriber transport.
// wire.go and mux.go are the load-bearing byte/socket contracts that clock (F)
// and transport (G) serialize against; do not reorder header fields.
package stream

import (
	"encoding/binary"
	"errors"
)

// Canonical PCM format — every frame on the wire is exactly this (§8.1).
const (
	SampleRate    = 48000                            // Hz
	Channels      = 2                                // stereo
	BytesPerSmpl  = 2                                // s16le, per channel
	FrameDuration = 20                               // ms
	FrameSamples  = 960                              // samples per channel per frame (48000*20/1000)
	FrameBytes    = 3840                             // FrameSamples * Channels * BytesPerSmpl
	FrameNanos    = int64(FrameDuration) * 1_000_000 // pts step, ns
)

// Packet types. 0x0x/0x1x are multiplexed on the member's STREAM_PORT UDP
// socket (§8.4); 0x2x are the data-plane stream-control types on the master's
// SOURCE_PORT (§8.7); 0x3x/0x40 are the v2 control plane (D49–D58,
// DUMB-CLIENT.md §6): master→playback commands on the playback node's
// CONTROL_PORT, and STATUS back to the master's SOURCE_PORT.
const (
	TypeAudio    byte = 0x01 // audio frame:  header + PCM/Opus payload
	TypeFEC      byte = 0x02 // XOR parity:   header + parity payload
	TypeClockReq byte = 0x10 // clock request (F)
	TypeClockRsp byte = 0x11 // clock reply   (F)

	TypeHello    byte = 0x20 // sub→src subscribe / keepalive; payload flag: prime-me (G)
	TypeBye      byte = 0x21 // sub→src "leaving, stop sending" (G)
	TypeRestart  byte = 0x22 // sub→src "got lost, re-prime and resume" (G)
	TypeReconfig byte = 0x23 // src→sub "gen/settings changed: resubscribe"; payload flag: stop (G)

	// v2 control plane (master→playback on CONTROL_PORT; idempotent soft-state, D58).
	TypeAttach    byte = 0x30 // master→pb "join this stream"; payload AttachPayload (control.go)
	TypeDetach    byte = 0x31 // master→pb "leave, go idle"; no payload
	TypeSetVol    byte = 0x32 // master→pb volume + mute; payload SetVolPayload
	TypeSetDelay  byte = 0x33 // master→pb output-delay ms (signed); payload SetDelayPayload
	TypeSetCap    byte = 0x34 // master→pb enable/disable a capability; payload SetCapPayload
	TypeSetEq     byte = 0x35 // master→pb cross-room device-buffer equalization delay ms (unsigned, added); payload SetEqualizePayload (D65)
	TypeStatus    byte = 0x40 // pb→master telemetry; payload StatusPayload
	TypeStatusReq byte = 0x41 // master→pb liveness poll "send STATUS now"; no payload (D60)
)

// Control payload flags (0x2x, 1 byte).
const (
	FlagPrimeMe byte = 0x01 // Hello: please burst-prime me
	FlagStop    byte = 0x01 // Reconfig: this is the stop / end-of-session notice
)

// Magic byte starting every framed packet (audio/fec/clock).
const Magic byte = 0xE5

// HeaderSize is the fixed on-wire size of Header, in bytes.
const HeaderSize = 24

// Header is the common frame header that precedes every payload.
//
// Byte layout (big-endian, offsets in bytes):
//
//	off size field        meaning
//	  0    1  Magic        0xE5, sanity / framing
//	  1    1  Type         packet type
//	  2    4  Gen          session generation (uint32); receivers drop stale gens
//	  6    8  Seq          frame sequence number (uint64), 0-based per session
//	 14    8  PTS          presentation timestamp, master-clock nanoseconds (int64)
//	 22    2  PayloadLen   payload byte count following the header (uint16)
//	 ----      total 24 bytes ----
type Header struct {
	Magic      byte
	Type       byte
	Gen        uint32
	Seq        uint64
	PTS        int64
	PayloadLen uint16
}

// Encode writes the 24-byte header into dst[:HeaderSize] (big-endian).
// Panics if len(dst) < HeaderSize. Returns HeaderSize for chaining.
func (h Header) Encode(dst []byte) int {
	_ = dst[HeaderSize-1] // bounds check / panic if too short
	dst[0] = h.Magic
	dst[1] = h.Type
	binary.BigEndian.PutUint32(dst[2:6], h.Gen)
	binary.BigEndian.PutUint64(dst[6:14], h.Seq)
	binary.BigEndian.PutUint64(dst[14:22], uint64(h.PTS))
	binary.BigEndian.PutUint16(dst[22:24], h.PayloadLen)
	return HeaderSize
}

// AppendFrame appends header+payload to dst and returns the grown slice.
// It sets PayloadLen = len(payload); caller need not pre-set it.
func (h Header) AppendFrame(dst, payload []byte) []byte {
	h.PayloadLen = uint16(len(payload))
	var hdr [HeaderSize]byte
	h.Encode(hdr[:])
	dst = append(dst, hdr[:]...)
	dst = append(dst, payload...)
	return dst
}

// Decode parses a 24-byte header from src. Does not validate Magic/Type.
// Returns ErrShort if len(src) < HeaderSize.
func Decode(src []byte) (Header, error) {
	if len(src) < HeaderSize {
		return Header{}, ErrShort
	}
	return Header{
		Magic:      src[0],
		Type:       src[1],
		Gen:        binary.BigEndian.Uint32(src[2:6]),
		Seq:        binary.BigEndian.Uint64(src[6:14]),
		PTS:        int64(binary.BigEndian.Uint64(src[14:22])),
		PayloadLen: binary.BigEndian.Uint16(src[22:24]),
	}, nil
}

// DecodeFrame parses header + exactly PayloadLen payload bytes from a single
// datagram (or a TCP length-framed chunk). Returns the header and a sub-slice
// of buf aliasing the payload (copy if retained). ErrShort / ErrBadMagic on
// malformed input.
func DecodeFrame(buf []byte) (Header, []byte, error) {
	h, err := Decode(buf)
	if err != nil {
		return Header{}, nil, err
	}
	if h.Magic != Magic {
		return Header{}, nil, ErrBadMagic
	}
	end := HeaderSize + int(h.PayloadLen)
	if len(buf) < end {
		return Header{}, nil, ErrShort
	}
	return h, buf[HeaderSize:end], nil
}

// XORInto computes parity for FEC: dst[i] ^= src[i] for i < len(src); shorter
// src is zero-padded (no-op past its end). dst must be FrameBytes for audio.
func XORInto(dst, src []byte) {
	n := len(src)
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] ^= src[i]
	}
}

var (
	// ErrShort means the buffer is shorter than the header or declared payload.
	ErrShort = errors.New("wire: buffer shorter than header")
	// ErrBadMagic means the framing magic byte did not match.
	ErrBadMagic = errors.New("wire: bad magic")
)
