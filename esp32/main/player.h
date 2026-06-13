// player.h — jitter buffer + master-clock playout (PLAYER §8). Buffers
// audio by seq, decodes opus in seq order at the playout deadline, writes one
// 20 ms frame per output slot, silence on gaps, skips frames already a full
// frame late. Drives the I2S sink and feeds the rate servo.
#pragma once
#include <stdbool.h>
#include <stdint.h>
#include "wire.h"
#include "config.h"

bool player_init(const ens_config_t *cfg);   // i2s + opus + volume + audio task

// (Re)arm for a session: reset the jitter buffer, set codec + playout lead, take
// the new generation. Called on ATTACH and on a gen change / non-stop RECONFIG.
void player_arm(uint8_t codec, uint32_t gen, int buffer_ms);
void player_detach(void);                     // idle: drop buffer, settle the line

// Feed one decoded-or-raw audio frame (net has already gen-matched it).
void player_push(uint64_t seq, int64_t pts, const uint8_t *payload, int len);

void player_set_buffer_ms(int ms);
void player_set_delay(int ms);   // device-latency comp (signed); re-anchors
void player_set_eq(int ms);      // cross-room equalization delay (added); re-anchors
void player_set_volume(uint8_t pct, bool mute);
void player_set_cap(uint8_t cap_id, bool on);

bool player_attached(void);
bool player_got_frame(void);     // any frame accepted since arm (hello-retry gate)
bool player_starved(void);       // no fresh frame for > 2 s (RESTART trigger)

void player_fill_status(wire_status_t *s);    // populate telemetry fields
