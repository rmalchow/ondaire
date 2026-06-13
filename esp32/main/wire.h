// wire.h — the ensemble v2 on-wire protocol (magic 0xE5), mirrored byte-for-byte
// from internal/stream/wire.go and specified in docs/PLAYER.md. All
// multi-byte fields are big-endian. Receivers ignore unknown packet types and
// drop datagrams whose first byte != 0xE5.
#pragma once
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define WIRE_MAGIC        0xE5
#define WIRE_HEADER_SIZE  24

// Canonical PCM frame: 48 kHz, stereo, s16le, 20 ms.
#define WIRE_SAMPLE_RATE  48000
#define WIRE_CHANNELS     2
#define WIRE_FRAME_SAMPLES 960                       // per channel per 20 ms
#define WIRE_FRAME_BYTES  3840                       // 960 * 2ch * 2B
#define WIRE_FRAME_NANOS  ((int64_t)20 * 1000000)    // pts step, ns

// Packet types.
enum {
    WIRE_AUDIO     = 0x01,
    WIRE_FEC       = 0x02,
    WIRE_CLOCK_REQ = 0x10,
    WIRE_CLOCK_RSP = 0x11,
    WIRE_HELLO     = 0x20,
    WIRE_BYE       = 0x21,
    WIRE_RESTART   = 0x22,
    WIRE_RECONFIG  = 0x23,
    WIRE_ATTACH    = 0x30,
    WIRE_DETACH    = 0x31,
    WIRE_SETVOL    = 0x32,
    WIRE_SETDELAY  = 0x33,
    WIRE_SETCAP    = 0x34,
    WIRE_SETEQ     = 0x35,
    WIRE_STATUS    = 0x40,
    WIRE_STATUSREQ = 0x41,
};

// Data-plane control payload flags (one byte).
#define WIRE_FLAG_PRIME 0x01   // HELLO/RESTART: please burst-prime me
#define WIRE_FLAG_STOP  0x01   // RECONFIG: end-of-session

// Codec / transport enums (ATTACH payload).
enum { WIRE_CODEC_PCM = 0, WIRE_CODEC_OPUS = 1 };
enum { WIRE_TRANSPORT_UDP = 0, WIRE_TRANSPORT_TCP = 1 };

#define WIRE_STATUS_LEN 87

// STATUS flag bits (§6.3).
#define WIRE_ST_SYNCED     0x01
#define WIRE_ST_PLAYING    0x02
#define WIRE_ST_CALIBRATED 0x04

typedef struct {
    uint8_t  magic;
    uint8_t  type;
    uint32_t gen;
    uint64_t seq;
    int64_t  pts;
    uint16_t payload_len;
} wire_header_t;

// ATTACH (§6.1) — 16 bytes.
typedef struct {
    uint32_t source_ip;   // IPv4, host byte order after decode
    uint16_t source_port;
    uint32_t clock_ip;
    uint16_t clock_port;
    uint8_t  codec;       // WIRE_CODEC_*
    uint8_t  transport;   // WIRE_TRANSPORT_*
    uint16_t buffer_ms;
} wire_attach_t;

// STATUS (§6.3) — 87 bytes.
typedef struct {
    uint8_t  node_id[16];
    uint8_t  flags;
    uint16_t buffered;
    uint64_t last_seq;
    int64_t  offset_ns;
    int64_t  rtt_ns;
    int32_t  rate_ppm_x1000;
    uint64_t played;
    uint64_t silence;
    uint64_t late;
    int64_t  device_delay_ns;
    int64_t  phase_err_ns;
} wire_status_t;

// Header encode/decode. enc writes WIRE_HEADER_SIZE bytes; returns size.
int  wire_header_encode(uint8_t *dst, const wire_header_t *h);
bool wire_header_decode(const uint8_t *src, size_t n, wire_header_t *out);

// Build a full frame (header + payload) into dst (cap bytes). Sets payload_len.
// Returns total bytes written, or -1 if dst too small.
int wire_frame_encode(uint8_t *dst, size_t cap, uint8_t type, uint32_t gen,
                      uint64_t seq, int64_t pts, const uint8_t *payload, size_t plen);

// Payload codecs. Decoders return false on a short/invalid payload.
bool wire_attach_decode(const uint8_t *p, size_t n, wire_attach_t *out);
int  wire_status_encode(uint8_t *p, size_t cap, const wire_status_t *s); // -> WIRE_STATUS_LEN
