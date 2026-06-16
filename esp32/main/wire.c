#include "wire.h"
#include <string.h>

// --- big-endian helpers ----------------------------------------------------
static inline void put_u16(uint8_t *b, uint16_t v) { b[0] = v >> 8; b[1] = v; }
static inline void put_u32(uint8_t *b, uint32_t v) {
    b[0] = v >> 24; b[1] = v >> 16; b[2] = v >> 8; b[3] = v;
}
static inline void put_u64(uint8_t *b, uint64_t v) {
    for (int i = 7; i >= 0; i--) { b[i] = v & 0xff; v >>= 8; }
}
static inline uint16_t get_u16(const uint8_t *b) { return ((uint16_t)b[0] << 8) | b[1]; }
static inline uint32_t get_u32(const uint8_t *b) {
    return ((uint32_t)b[0] << 24) | ((uint32_t)b[1] << 16) | ((uint32_t)b[2] << 8) | b[3];
}
static inline uint64_t get_u64(const uint8_t *b) {
    uint64_t v = 0;
    for (int i = 0; i < 8; i++) v = (v << 8) | b[i];
    return v;
}

int wire_header_encode(uint8_t *dst, const wire_header_t *h) {
    dst[0] = h->magic;
    dst[1] = h->type;
    put_u32(dst + 2, h->gen);
    put_u64(dst + 6, h->seq);
    put_u64(dst + 14, (uint64_t)h->pts);
    put_u16(dst + 22, h->payload_len);
    return WIRE_HEADER_SIZE;
}

bool wire_header_decode(const uint8_t *src, size_t n, wire_header_t *out) {
    if (n < WIRE_HEADER_SIZE) return false;
    out->magic = src[0];
    out->type = src[1];
    out->gen = get_u32(src + 2);
    out->seq = get_u64(src + 6);
    out->pts = (int64_t)get_u64(src + 14);
    out->payload_len = get_u16(src + 22);
    return true;
}

int wire_frame_encode(uint8_t *dst, size_t cap, uint8_t type, uint32_t gen,
                      uint64_t seq, int64_t pts, const uint8_t *payload, size_t plen) {
    if (cap < WIRE_HEADER_SIZE + plen) return -1;
    wire_header_t h = {
        .magic = WIRE_MAGIC, .type = type, .gen = gen,
        .seq = seq, .pts = pts, .payload_len = (uint16_t)plen,
    };
    wire_header_encode(dst, &h);
    if (plen) memcpy(dst + WIRE_HEADER_SIZE, payload, plen);
    return (int)(WIRE_HEADER_SIZE + plen);
}

bool wire_attach_decode(const uint8_t *p, size_t n, wire_attach_t *out) {
    if (n < 16) return false;
    out->source_ip = get_u32(p + 0);
    out->source_port = get_u16(p + 4);
    out->clock_ip = get_u32(p + 6);
    out->clock_port = get_u16(p + 10);
    out->codec = p[12];
    out->transport = p[13];
    out->buffer_ms = get_u16(p + 14);
    return true;
}

int wire_status_encode(uint8_t *p, size_t cap, const wire_status_t *s) {
    if (cap < WIRE_STATUS_LEN) return -1;
    memcpy(p + 0, s->node_id, 16);
    p[16] = s->flags;
    put_u16(p + 17, s->buffered);
    put_u64(p + 19, s->last_seq);
    put_u64(p + 27, (uint64_t)s->offset_ns);
    put_u64(p + 35, (uint64_t)s->rtt_ns);
    put_u32(p + 43, (uint32_t)s->rate_ppm_x1000);
    put_u64(p + 47, s->played);
    put_u64(p + 55, s->silence);
    put_u64(p + 63, s->late);
    put_u64(p + 71, (uint64_t)s->device_delay_ns);
    put_u64(p + 79, (uint64_t)s->phase_err_ns);
    put_u64(p + 87, s->samples_injected);
    put_u64(p + 95, s->samples_dropped);
    return WIRE_STATUS_LEN;
}
