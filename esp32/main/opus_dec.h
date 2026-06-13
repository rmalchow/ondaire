// opus_dec.h — decode-only Opus wrapper (PLAYER §10). The stream is
// 48 kHz / stereo / 20 ms packets (960 samples/ch). Reset the decoder on every
// generation change; on a lost packet use PLC (decode a null packet).
#pragma once
#include <stdbool.h>
#include <stdint.h>

bool opus_dec_init(void);        // create the 48k/stereo decoder
void opus_dec_reset(void);       // reset state on a generation change

// Decode one packet into pcm (must hold >= WIRE_FRAME_BYTES). Returns samples
// per channel decoded (960 on success), or -1 on error.
int  opus_dec_decode(const uint8_t *pkt, int len, int16_t *pcm);

// Packet-loss concealment: synthesize one 20 ms frame for a missing packet.
int  opus_dec_plc(int16_t *pcm);
