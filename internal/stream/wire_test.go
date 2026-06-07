package stream

import (
	"encoding/binary"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	cases := []Header{
		{Magic: Magic, Type: TypeAudio, Gen: 0, Seq: 0, PTS: 0, PayloadLen: 0},
		{Magic: Magic, Type: TypeFEC, Gen: 1, Seq: 1, PTS: 1, PayloadLen: FrameBytes},
		{Magic: Magic, Type: TypeClockReq, Gen: ^uint32(0), Seq: ^uint64(0), PTS: -1, PayloadLen: ^uint16(0)},
		{Magic: Magic, Type: TypeAudio, Gen: 12345, Seq: 1 << 40, PTS: 1 << 50, PayloadLen: 1234},
	}
	for _, h := range cases {
		var buf [HeaderSize]byte
		h.Encode(buf[:])
		got, err := Decode(buf[:])
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got != h {
			t.Fatalf("round-trip: got %+v want %+v", got, h)
		}
	}
}

func TestHeaderBigEndianLayout(t *testing.T) {
	h := Header{Magic: Magic, Type: 0x42, Gen: 0x01020304, Seq: 0x0102030405060708, PTS: 0x1122334455667788, PayloadLen: 0xABCD}
	var b [HeaderSize]byte
	h.Encode(b[:])
	if b[0] != Magic || b[1] != 0x42 {
		t.Fatalf("magic/type bytes wrong: %x %x", b[0], b[1])
	}
	if binary.BigEndian.Uint32(b[2:6]) != 0x01020304 {
		t.Fatal("gen offset wrong")
	}
	if binary.BigEndian.Uint64(b[6:14]) != 0x0102030405060708 {
		t.Fatal("seq offset wrong")
	}
	if binary.BigEndian.Uint64(b[14:22]) != 0x1122334455667788 {
		t.Fatal("pts offset wrong")
	}
	if binary.BigEndian.Uint16(b[22:24]) != 0xABCD {
		t.Fatal("payloadLen offset wrong")
	}
}

func TestHeaderSizeConstant(t *testing.T) {
	if HeaderSize != 24 {
		t.Fatalf("HeaderSize = %d, want 24", HeaderSize)
	}
	var b [HeaderSize]byte
	if n := (Header{}).Encode(b[:]); n != 24 {
		t.Fatalf("Encode returned %d", n)
	}
}

func TestEncodeShortBufferPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Encode into 23-byte buffer did not panic")
		}
	}()
	var b [HeaderSize - 1]byte
	(Header{}).Encode(b[:])
}

func TestDecodeShortBuffer(t *testing.T) {
	if _, err := Decode(make([]byte, HeaderSize-1)); err != ErrShort {
		t.Fatalf("want ErrShort, got %v", err)
	}
}

func TestDecodeFrameBadMagic(t *testing.T) {
	buf := make([]byte, HeaderSize)
	buf[0] = 0x00 // wrong magic
	if _, _, err := DecodeFrame(buf); err != ErrBadMagic {
		t.Fatalf("want ErrBadMagic, got %v", err)
	}
}

func TestDecodeFrameTruncatedPayload(t *testing.T) {
	h := Header{Magic: Magic, Type: TypeAudio, PayloadLen: 100}
	var hdr [HeaderSize]byte
	h.Encode(hdr[:])
	buf := append(hdr[:], make([]byte, 50)...) // only 50 of 100 payload bytes
	if _, _, err := DecodeFrame(buf); err != ErrShort {
		t.Fatalf("want ErrShort, got %v", err)
	}
}

func TestAppendFrameSetsPayloadLen(t *testing.T) {
	h := Header{Magic: Magic, Type: TypeAudio, Seq: 5}
	payload := []byte("hello world payload")
	frame := h.AppendFrame(nil, payload)
	gotH, gotP, err := DecodeFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if gotH.PayloadLen != uint16(len(payload)) {
		t.Fatalf("PayloadLen = %d, want %d", gotH.PayloadLen, len(payload))
	}
	if string(gotP) != string(payload) {
		t.Fatalf("payload mismatch: %q", gotP)
	}
	if gotH.Seq != 5 {
		t.Fatal("seq not preserved")
	}
}

func TestXORIntoPadsShortSrc(t *testing.T) {
	dst := make([]byte, 5)
	for i := range dst {
		dst[i] = byte(i + 1)
	}
	XORInto(dst, []byte{0xFF, 0xFF}) // shorter src
	if dst[0] != (1^0xFF) || dst[1] != (2^0xFF) {
		t.Fatal("xor of short src wrong")
	}
	if dst[2] != 3 || dst[3] != 4 || dst[4] != 5 {
		t.Fatal("padding past src not a no-op")
	}
	// longer src than dst must not panic
	XORInto(make([]byte, 2), make([]byte, 10))
}

func TestPCMConstants(t *testing.T) {
	if FrameBytes != FrameSamples*Channels*BytesPerSmpl {
		t.Fatalf("FrameBytes = %d", FrameBytes)
	}
	if FrameBytes != 3840 {
		t.Fatalf("FrameBytes != 3840")
	}
	if FrameNanos != 20_000_000 {
		t.Fatalf("FrameNanos = %d", FrameNanos)
	}
}
